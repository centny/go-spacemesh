package node

import (
	"context"
	"fmt"

	"github.com/spacemeshos/go-spacemesh/activation"
	"github.com/spacemeshos/go-spacemesh/api/grpcserver"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/log"
	"github.com/spacemeshos/go-spacemesh/syncer"
)

func (app *App) initSmeshingGroup(ctx context.Context, lg log.Log, goldenATXID types.ATXID, poetDb *activation.PoetDb, layersPerEpoch uint32, newSyncer *syncer.Syncer) {
	app.postSetupGroup = map[string]*activation.PostSetupManager{}
	app.atxBuilderGroup = map[string]*activation.Builder{}
	for i, opts := range app.Config.SMESHING.OptsGroup {
		defaultOpts := activation.DefaultPostSetupOpts()
		opts.Throttle = defaultOpts.Throttle
		opts.Scrypt = defaultOpts.Scrypt
		opts.ComputeBatchSize = defaultOpts.ComputeBatchSize
		postSetupMgr, err := activation.NewPostSetupManager(
			app.edSgn.NodeID(),
			app.Config.POST,
			app.addLogger(PostLogger, lg),
			app.cachedDB, goldenATXID,
			app.Config.SMESHING.ProvingOpts,
		)
		if err != nil {
			app.log.Panic("failed to create post setup manager: %v", err)
		}

		nipostBuilder, err := activation.NewNIPostBuilder(
			app.edSgn.NodeID(),
			postSetupMgr,
			poetDb,
			app.Config.PoETServers,
			opts.DataDir,
			app.addLogger(NipostBuilderLogger, lg),
			app.edSgn,
			app.Config.POET,
			app.clock,
			activation.WithNipostValidator(app.validator),
		)
		if err != nil {
			app.log.Panic("failed to create nipost builder: %v", err)
		}

		coinbaseAddr, err := types.StringToAddress(opts.CoinbaseAccount)
		if err != nil {
			app.log.Panic("failed to parse CoinbaseAccount address `%s`: %v", opts.CoinbaseAccount, err)
		}
		if coinbaseAddr.IsEmpty() {
			app.log.Panic("invalid coinbase account")
		}

		builderConfig := activation.Config{
			CoinbaseAccount:  coinbaseAddr,
			GoldenATXID:      goldenATXID,
			LayersPerEpoch:   layersPerEpoch,
			RegossipInterval: app.Config.RegossipAtxInterval,
		}
		atxBuilder := activation.NewBuilder(
			builderConfig,
			app.edSgn.NodeID(),
			app.edSgn,
			app.cachedDB,
			app.host,
			nipostBuilder,
			postSetupMgr,
			app.clock,
			newSyncer,
			app.addLogger(fmt.Sprintf("atxBuilder-%v", i), lg),
			activation.WithContext(ctx),
			activation.WithPoetConfig(app.Config.POET),
			activation.WithPoetRetryInterval(app.Config.HARE.WakeupDelta),
			activation.WithValidator(app.validator),
		)

		app.postSetupGroup[i] = postSetupMgr
		app.atxBuilderGroup[i] = atxBuilder
	}
}

func (app *App) startServicesGroup() {
	for i, opts := range app.Config.SMESHING.OptsGroup {
		defaultOpts := activation.DefaultPostSetupOpts()
		opts.Throttle = defaultOpts.Throttle
		opts.Scrypt = defaultOpts.Scrypt
		opts.ComputeBatchSize = defaultOpts.ComputeBatchSize
		coinbaseAddr, err := types.StringToAddress(opts.CoinbaseAccount)
		if err != nil {
			app.log.Panic(
				"failed to parse CoinbaseAccount address on start `%s`: %v",
				app.Config.SMESHING.CoinbaseAccount,
				err,
			)
		}
		if err := app.atxBuilderGroup[i].StartSmeshing(coinbaseAddr, opts); err != nil {
			app.log.Panic("failed to start smeshing: %v", err)
		}
	}
}

func (app *App) stopServicesGroup() {
	for _, atxBuilder := range app.atxBuilderGroup {
		_ = atxBuilder.StopSmeshing(false)
	}
}

func (app *App) startAPIServicesGroup(ctx context.Context, logger log.Log) error {
	app.grpcGroupService = map[string]*grpcserver.Server{}
	for i, opts := range app.Config.SMESHING.OptsGroup {
		defaultOpts := activation.DefaultPostSetupOpts()
		opts.Throttle = defaultOpts.Throttle
		opts.Scrypt = defaultOpts.Scrypt
		opts.ComputeBatchSize = defaultOpts.ComputeBatchSize
		app.grpcGroupService[i] = app.newGrpc(logger, app.Config.API.GroupListener[i])
		gsvc := grpcserver.NewSmesherService(
			app.postSetupGroup[i],
			app.atxBuilderGroup[i],
			app.Config.API.SmesherStreamInterval,
			opts,
		)
		gsvc.RegisterService(app.grpcGroupService[i])
		if err := app.grpcGroupService[i].Start(); err != nil {
			return err
		}
	}
	return nil
}
