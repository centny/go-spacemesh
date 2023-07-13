package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	varint "github.com/multiformats/go-varint"
	poetconfig "github.com/spacemeshos/poet/config"
	"github.com/spacemeshos/poet/server"
	postCfg "github.com/spacemeshos/post/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/spacemeshos/go-spacemesh/codec"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/config"
	"github.com/spacemeshos/go-spacemesh/log"
	"github.com/spacemeshos/go-spacemesh/log/logtest"
	ps "github.com/spacemeshos/go-spacemesh/p2p/pubsub"
	"github.com/spacemeshos/post/initialization"
)

func TestPeerDisconnectForMessageResultValidationReject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := logtest.New(t)

	// Make 2 node instances
	conf1 := config.DefaultTestConfig()
	conf1.DataDirParent = t.TempDir()
	conf1.FileLock = filepath.Join(conf1.DataDirParent, "LOCK")
	conf1.P2P.Listen = "/ip4/127.0.0.1/tcp/0"
	app1, err := NewApp(&conf1, l)
	require.NoError(t, err)
	conf2 := config.DefaultTestConfig()
	// We need to copy the genesis config to ensure that both nodes share the
	// same gnenesis ID, otherwise they will not be able to connect to each
	// other.
	*conf2.Genesis = *conf1.Genesis
	conf2.DataDirParent = t.TempDir()
	conf2.FileLock = filepath.Join(conf2.DataDirParent, "LOCK")
	conf2.P2P.Listen = "/ip4/127.0.0.1/tcp/0"
	app2, err := NewApp(&conf2, l)
	require.NoError(t, err)

	types.SetLayersPerEpoch(conf1.LayersPerEpoch)
	t.Cleanup(func() {
		app1.Cleanup(ctx)
		app2.Cleanup(ctx)
	})
	g := errgroup.Group{}
	g.Go(func() error {
		return app1.Start(ctx)
	})
	<-app1.Started()
	g.Go(func() error {
		return app2.Start(ctx)
	})
	<-app2.Started()

	// Connect app2 to app1
	err = app2.Host().Connect(context.Background(), peer.AddrInfo{
		ID:    app1.Host().ID(),
		Addrs: app1.Host().Addrs(),
	})
	require.NoError(t, err)

	conns := app2.Host().Network().ConnsToPeer(app1.Host().ID())
	require.Equal(t, 1, len(conns))

	// Wait for streams to be established, one outbound and one inbound.
	require.Eventually(t, func() bool {
		return len(conns[0].GetStreams()) == 2
	}, time.Second*5, time.Millisecond*50)

	s := getStream(conns[0], pubsub.GossipSubID_v11, network.DirOutbound)

	require.True(t, app1.syncer.IsSynced(ctx))
	require.True(t, app2.syncer.IsSynced(ctx))

	protocol := ps.ProposalProtocol
	// Send a message that doesn't result in ValidationReject.
	p := types.Proposal{}
	bytes, err := codec.Encode(&p)
	require.NoError(t, err)
	m := &pubsubpb.Message{
		Data:  bytes,
		Topic: &protocol,
	}
	err = writeRpc(rpcWithMessages(m), s)
	require.NoError(t, err)

	// Verify that connections remain up
	for i := 0; i < 5; i++ {
		conns := app2.Host().Network().ConnsToPeer(app1.Host().ID())
		require.Equal(t, 1, len(conns))
		time.Sleep(100 * time.Millisecond)
	}

	// Send message that results in ValidationReject
	m = &pubsubpb.Message{
		Data:  make([]byte, 20),
		Topic: &protocol,
	}
	err = writeRpc(rpcWithMessages(m), s)
	require.NoError(t, err)

	// Wait for connection to be dropped
	require.Eventually(t, func() bool {
		return len(app2.Host().Network().ConnsToPeer(app1.Host().ID())) == 0
	}, time.Second*15, time.Millisecond*200)

	// Stop the nodes by canceling the context
	cancel()
	// Wait for nodes to finish
	require.NoError(t, g.Wait())
}

func TestConsensus(t *testing.T) {
	spew.Config.DisableMethods = true
	cfg := fastnet()
	cfg.Genesis = config.DefaultGenesisConfig()
	cfg.SMESHING.Start = true
	// cfg := config.DefaultTestConfig()
	// cfg.LayerDuration = time.Second * 10
	// cfg.HARE.RoundDuration = time.Second
	// cfg.Beacon.GracePeriodDuration = 0
	// cfg.Beacon.ProposalDuration = time.Second * 5
	// cfg.Beacon.FirstVotingRoundDuration = time.Second * 5
	// cfg.Beacon.VotingRoundDuration = time.Second * 5
	// cfg.Beacon.WeakCoinRoundDuration = time.Second * 5
	l := logtest.New(t)
	_, cleanup, err := NewNetwork(cfg, l, 2)
	require.NoError(t, err)
	time.Sleep(time.Second * 600)
	require.NoError(t, cleanup())
}

func NewNetwork(conf config.Config, l log.Log, size int) ([]*App, func() error, error) {
	// We need to set this global state
	types.SetLayersPerEpoch(conf.LayersPerEpoch)
	types.SetNetworkHRP(conf.NetworkHRP) // set to generate coinbase

	ctx, cancel := context.WithCancel(context.Background())
	g := errgroup.Group{}
	var apps []*App
	var datadirs []string
	cleanup := func() error {
		// cancel the context
		cancel()
		// Wait for nodes to shutdown
		g.Wait()
		// Clean their datadirs
		for _, d := range datadirs {
			err := os.RemoveAll(d)
			if err != nil {
				return err
			}
		}
		return nil
	}

	poet, poetDir, err := NewPoet(poetconfig.DefaultConfig(), &conf)
	if err != nil {
		return nil, nil, err
	}
	g.Go(func() error {
		return poet.Start(ctx)
	})
	datadirs = append(datadirs, poetDir)

	// Add the poet address to the config
	conf.PoETServers = []string{"http://" + poet.GrpcRestProxyAddr().String()}

	// We encode and decode the config in order to deep copy it.
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(conf)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	marshaled := buf.Bytes()

	for i := 0; i < size; i++ {
		var c config.Config
		err := gob.NewDecoder(bytes.NewBuffer(marshaled)).Decode(&c)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		dir, err := os.MkdirTemp("", "")
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		datadirs = append(datadirs, dir)

		c.DataDirParent = dir
		c.SMESHING.Opts.DataDir = dir
		fmt.Println(dir)
		fmt.Println(conf.SMESHING.Opts.DataDir)
		c.FileLock = filepath.Join(c.DataDirParent, "LOCK")
		c.P2P.Listen = "/ip4/127.0.0.1/tcp/0"
		c.API.PublicListener = "0.0.0.0:0"
		c.API.PrivateListener = "0.0.0.0:0"
		c.API.JSONListener = "0.0.0.0:0"

		app, err := NewApp(&c, l)
		if err != nil {
			cleanup()
			return nil, nil, err
		}

		g.Go(func() error {
			return app.Start(ctx)
		})
		<-app.Started()
		apps = append(apps, app)
	}

	// Connect all nodes to each other
	for i := 0; i < size; i++ {
		for j := i + 1; j < size; j++ {
			err = apps[i].Host().Connect(context.Background(), peer.AddrInfo{
				ID:    apps[j].Host().ID(),
				Addrs: apps[j].Host().Addrs(),
			})
			if err != nil {
				cleanup()
				return nil, nil, err
			}
		}
	}
	return apps, cleanup, nil
}

func NewApp(conf *config.Config, l log.Log) (*App, error) {
	app := New(
		WithConfig(conf),
		WithLog(l),
	)

	var err error
	if err = app.Initialize(); err != nil {
		return nil, err
	}

	/* Create or load miner identity */
	if app.edSgn, err = app.LoadOrCreateEdSigner(); err != nil {
		return app, fmt.Errorf("could not retrieve identity: %w", err)
	}
	// app.edSgn, err = signing.NewEdSigner()
	// if err != nil {
	// 	return nil, err
	// }
	return app, err
}

func NewPoet(cfg *poetconfig.Config, appConf *config.Config) (*server.Server, string, error) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, "", err
	}
	cfg.PoetDir = dir
	cfg.DataDir = filepath.Join(dir, "data")
	cfg.LogDir = filepath.Join(dir, "logs")

	cfg.RawRESTListener = "0.0.0.0:0"
	cfg.RawRPCListener = "0.0.0.0:0"
	cfg.Service.Genesis.UnmarshalFlag(appConf.Genesis.GenesisTime)
	cfg.Service.EpochDuration = appConf.LayerDuration * time.Duration(appConf.LayersPerEpoch)
	cfg.Service.CycleGap = appConf.POET.CycleGap
	cfg.Service.PhaseShift = appConf.POET.PhaseShift
	srv, err := server.New(context.Background(), *cfg)
	if err != nil {
		err := fmt.Errorf("poet init faiure: %w", err)
		errDir := os.RemoveAll(dir)
		if errDir != nil {
			return nil, "", fmt.Errorf("failed to remove poet dir %q after poet init failure: %w", errDir, err)
		}
		return nil, "", err
	}

	return srv, dir, nil
	// app.log.With().Warning("lauching poet in standalone mode", log.Any("config", cfg))

	// app.eg.Go(func() error {
	// 	if err := srv.Start(ctx); err != nil {
	// 		app.log.With().Error("poet server failed", log.Err(err))
	// 		return err
	// 	}
	// 	return nil
	// })
}

func getStream(c network.Conn, p protocol.ID, dir network.Direction) network.Stream {
	for _, s := range c.GetStreams() {
		if s.Protocol() == p && s.Stat().Direction == dir {
			return s
		}
	}
	return nil
}

func rpcWithMessages(msgs ...*pubsubpb.Message) *pubsub.RPC {
	return &pubsub.RPC{RPC: pubsubpb.RPC{Publish: msgs}}
}

func writeRpc(rpc *pubsub.RPC, s network.Stream) error {
	size := uint64(rpc.Size())

	buf := make([]byte, varint.UvarintSize(size)+int(size))

	n := binary.PutUvarint(buf, size)
	_, err := rpc.MarshalTo(buf[n:])
	if err != nil {
		return err
	}

	_, err = s.Write(buf)
	return err
}

func fastnet() config.Config {
	conf := config.DefaultConfig()
	// conf.Address = types.DefaultTestAddressConfig()
	conf.NetworkHRP = "stest"

	conf.BaseConfig.OptFilterThreshold = 90

	conf.HARE.N = 800
	conf.HARE.ExpectedLeaders = 10
	conf.HARE.LimitConcurrent = 5
	conf.HARE.LimitIterations = 3
	conf.HARE.RoundDuration = 2 * time.Second
	conf.HARE.WakeupDelta = 3 * time.Second

	conf.P2P.MinPeers = 10

	conf.Genesis = &config.GenesisConfig{
		ExtraData: "fastnet",
	}

	conf.LayerAvgSize = 50
	conf.LayerDuration = 15 * time.Second
	conf.Sync.Interval = 5 * time.Second
	conf.LayersPerEpoch = 4

	conf.POET.PhaseShift = 15 * time.Second
	conf.POET.GracePeriod = 2 * time.Second
	conf.POET.CycleGap = 10 * time.Second

	conf.Tortoise.Hdist = 4
	conf.Tortoise.Zdist = 2
	conf.Tortoise.BadBeaconVoteDelayLayers = 2

	conf.HareEligibility.ConfidenceParam = 2

	conf.POST.K1 = 12
	conf.POST.K2 = 4
	conf.POST.K3 = 4
	conf.POST.LabelsPerUnit = 32
	conf.POST.MaxNumUnits = 2
	conf.POST.MinNumUnits = 1

	conf.SMESHING.CoinbaseAccount = types.GenerateAddress([]byte("1")).String()
	conf.SMESHING.Start = false
	conf.SMESHING.Opts.ProviderID = int(initialization.CPUProviderID())
	conf.SMESHING.Opts.NumUnits = 2
	conf.SMESHING.Opts.Throttle = true
	// Override proof of work flags to use light mode (less memory intensive)
	conf.SMESHING.ProvingOpts.Flags = postCfg.RecommendedPowFlags()

	conf.Beacon.Kappa = 40
	conf.Beacon.Theta = big.NewRat(1, 4)
	conf.Beacon.FirstVotingRoundDuration = 10 * time.Second
	conf.Beacon.GracePeriodDuration = 30 * time.Second
	conf.Beacon.ProposalDuration = 2 * time.Second
	conf.Beacon.VotingRoundDuration = 2 * time.Second
	conf.Beacon.WeakCoinRoundDuration = 2 * time.Second
	conf.Beacon.RoundsNumber = 4
	conf.Beacon.BeaconSyncWeightUnits = 5
	conf.Beacon.VotesLimit = 100

	return conf
}

// func fastnet() config.Config {
// 	conf := config.DefaultConfig()
// 	conf.Address = types.DefaultTestAddressConfig()

// 	conf.BaseConfig.OptFilterThreshold = 90

// 	conf.HARE.N = 800
// 	conf.HARE.ExpectedLeaders = 10
// 	conf.HARE.LimitConcurrent = 5
// 	conf.HARE.LimitIterations = 3
// 	conf.HARE.RoundDuration = 2 * time.Second
// 	conf.HARE.WakeupDelta = 3 * time.Second

// 	conf.P2P.MinPeers = 10

// 	conf.Genesis = &config.GenesisConfig{
// 		ExtraData: "fastnet",
// 	}

// 	conf.LayerAvgSize = 50
// 	conf.LayerDuration = 15 * time.Second
// 	conf.Sync.Interval = 5 * time.Second
// 	conf.LayersPerEpoch = 4

// 	conf.Tortoise.Hdist = 4
// 	conf.Tortoise.Zdist = 2
// 	conf.Tortoise.BadBeaconVoteDelayLayers = 2

// 	conf.HareEligibility.ConfidenceParam = 2

// 	conf.POST.K1 = 12
// 	conf.POST.K2 = 4
// 	conf.POST.K3 = 4
// 	conf.POST.LabelsPerUnit = 128
// 	conf.POST.MaxNumUnits = 4
// 	conf.POST.MinNumUnits = 2

// 	conf.SMESHING.CoinbaseAccount = types.GenerateAddress([]byte("1")).String()
// 	conf.SMESHING.Start = false
// 	conf.SMESHING.Opts.ProviderID = int(initialization.CPUProviderID())
// 	conf.SMESHING.Opts.NumUnits = 2
// 	conf.SMESHING.Opts.Throttle = true
// 	// Override proof of work flags to use light mode (less memory intensive)
// 	conf.SMESHING.ProvingOpts.Flags = postCfg.RecommendedPowFlags()

// 	conf.Beacon.Kappa = 40
// 	conf.Beacon.Theta = big.NewRat(1, 4)
// 	conf.Beacon.FirstVotingRoundDuration = 10 * time.Second
// 	conf.Beacon.GracePeriodDuration = 30 * time.Second
// 	conf.Beacon.ProposalDuration = 2 * time.Second
// 	conf.Beacon.VotingRoundDuration = 2 * time.Second
// 	conf.Beacon.WeakCoinRoundDuration = 2 * time.Second
// 	conf.Beacon.RoundsNumber = 4
// 	conf.Beacon.BeaconSyncWeightUnits = 10
// 	conf.Beacon.VotesLimit = 100

// 	return conf
// }

// func (app *App) launchStandalone(ctx context.Context) error {
// 	if !app.Config.Standalone {
// 		return nil
// 	}
// 	if len(app.Config.PoETServers) != 1 {
// 		return fmt.Errorf("to launch in a standalone mode provide single local address for poet: %v", app.Config.PoETServers)
// 	}
// 	value := types.Beacon{}
// 	genesis := app.Config.Genesis.GenesisID()
// 	copy(value[:], genesis[:])
// 	epoch := types.GetEffectiveGenesis().GetEpoch() + 1
// 	app.log.With().Warning("using standalone mode for bootstrapping beacon",
// 		log.Uint32("epoch", epoch.Uint32()),
// 		log.Stringer("beacon", value),
// 	)
// 	if err := app.beaconProtocol.UpdateBeacon(epoch, value); err != nil {
// 		return fmt.Errorf("update standalone beacon: %w", err)
// 	}
// 	cfg := poetconfig.DefaultConfig()
// 	cfg.PoetDir = filepath.Join(app.Config.DataDir(), "poet")
// 	cfg.DataDir = cfg.PoetDir
// 	cfg.LogDir = cfg.PoetDir
// 	parsed, err := url.Parse(app.Config.PoETServers[0])
// 	if err != nil {
// 		return err
// 	}
// 	cfg.RawRESTListener = parsed.Host
// 	cfg.Service.Genesis.UnmarshalFlag(app.Config.Genesis.GenesisTime)
// 	cfg.Service.EpochDuration = app.Config.LayerDuration * time.Duration(app.Config.LayersPerEpoch)
// 	cfg.Service.CycleGap = app.Config.POET.CycleGap
// 	cfg.Service.PhaseShift = app.Config.POET.PhaseShift
// 	srv, err := server.New(ctx, *cfg)
// 	if err != nil {
// 		return fmt.Errorf("init poet server: %w", err)
// 	}
// 	app.log.With().Warning("lauching poet in standalone mode", log.Any("config", cfg))
// 	app.eg.Go(func() error {
// 		if err := srv.Start(ctx); err != nil {
// 			app.log.With().Error("poet server failed", log.Err(err))
// 			return err
// 		}
// 		return nil
// 	})
// 	return nil
// }
