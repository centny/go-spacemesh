package mesh

import (
	"bytes"
	"container/list"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sync"

	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/database"
	"github.com/spacemeshos/go-spacemesh/log"
	"github.com/spacemeshos/go-spacemesh/pendingtxs"
)

type layerMutex struct {
	m            sync.Mutex
	layerWorkers uint32
}

// DB represents a mesh database instance
type DB struct {
	log.Log
	blockCache            blockCache
	layers                database.Database
	blocks                database.Database
	transactions          database.Database
	contextualValidity    database.Database
	general               database.Database
	unappliedTxs          database.Database
	inputVector           database.Database
	unappliedTxsMutex     sync.Mutex
	blockMutex            sync.RWMutex
	orphanBlocks          map[types.LayerID]map[types.BlockID]struct{}
	coinflips             map[types.LayerID]bool // weak coinflip results from Hare
	layerMutex            map[types.LayerID]*layerMutex
	lhMutex               sync.Mutex
	InputVectorBackupFunc func(id types.LayerID) ([]types.BlockID, error)
	exit                  chan struct{}
}

// NewPersistentMeshDB creates an instance of a mesh database
func NewPersistentMeshDB(path string, blockCacheSize int, logger log.Log) (*DB, error) {
	bdb, err := database.NewLDBDatabase(filepath.Join(path, "blocks"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize blocks db: %v", err)
	}
	ldb, err := database.NewLDBDatabase(filepath.Join(path, "layers"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize layers db: %v", err)
	}
	vdb, err := database.NewLDBDatabase(filepath.Join(path, "validity"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize validity db: %v", err)
	}
	tdb, err := database.NewLDBDatabase(filepath.Join(path, "transactions"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transactions db: %v", err)
	}
	gdb, err := database.NewLDBDatabase(filepath.Join(path, "general"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize general db: %v", err)
	}
	utx, err := database.NewLDBDatabase(filepath.Join(path, "unappliedTxs"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize mesh unappliedTxs db: %v", err)
	}
	iv, err := database.NewLDBDatabase(filepath.Join(path, "inputvector"), 0, 0, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize mesh unappliedTxs db: %v", err)
	}

	ll := &DB{
		Log:                logger,
		blockCache:         newBlockCache(blockCacheSize * layerSize),
		blocks:             bdb,
		layers:             ldb,
		transactions:       tdb,
		general:            gdb,
		contextualValidity: vdb,
		unappliedTxs:       utx,
		inputVector:        iv,
		orphanBlocks:       make(map[types.LayerID]map[types.BlockID]struct{}),
		coinflips:          make(map[types.LayerID]bool),
		layerMutex:         make(map[types.LayerID]*layerMutex),
		exit:               make(chan struct{}),
	}
	for _, blk := range GenesisLayer().Blocks() {
		ll.Log.With().Info("Adding genesis block ", blk.ID(), blk.LayerIndex)
		if err := ll.AddBlock(blk); err != nil {
			ll.Log.With().Error("Error inserting genesis block to db", blk.ID(), blk.LayerIndex)
		}
		if err := ll.SaveContextualValidity(blk.ID(), true); err != nil {
			ll.Log.With().Error("Error inserting genesis block to db", blk.ID(), blk.LayerIndex)
		}
	}
	if err := ll.SaveLayerInputVectorByID(context.Background(), GenesisLayer().Index(), types.BlockIDs(GenesisLayer().Blocks())); err != nil {
		log.With().Error("Error inserting genesis input vector to db", GenesisLayer().Index())
	}
	return ll, err
}

// PersistentData checks to see if db is empty
func (m *DB) PersistentData() bool {
	if _, err := m.general.Get(constLATEST); err == nil {
		m.Info("found data to recover on disk")
		return true
	}
	m.Info("did not find data to recover on disk")
	return false
}

// NewMemMeshDB is a mock used for testing
func NewMemMeshDB(log log.Log) *DB {
	ll := &DB{
		Log:                log,
		blockCache:         newBlockCache(100 * layerSize),
		blocks:             database.NewMemDatabase(),
		layers:             database.NewMemDatabase(),
		general:            database.NewMemDatabase(),
		contextualValidity: database.NewMemDatabase(),
		transactions:       database.NewMemDatabase(),
		unappliedTxs:       database.NewMemDatabase(),
		inputVector:        database.NewMemDatabase(),
		orphanBlocks:       make(map[types.LayerID]map[types.BlockID]struct{}),
		coinflips:          make(map[types.LayerID]bool),
		layerMutex:         make(map[types.LayerID]*layerMutex),
		exit:               make(chan struct{}),
	}
	for _, blk := range GenesisLayer().Blocks() {
		ll.AddBlock(blk)
		ll.SaveContextualValidity(blk.ID(), true)
	}
	if err := ll.SaveLayerInputVectorByID(context.Background(), GenesisLayer().Index(), types.BlockIDs(GenesisLayer().Blocks())); err != nil {
		log.With().Error("Error inserting genesis input vector to db", GenesisLayer().Index())
	}
	return ll
}

// Close closes all resources
func (m *DB) Close() {
	close(m.exit)
	m.blocks.Close()
	m.layers.Close()
	m.transactions.Close()
	m.unappliedTxs.Close()
	m.inputVector.Close()
	m.general.Close()
	m.contextualValidity.Close()
}

// todo: for now, these methods are used to export dbs to sync, think about merging the two packages

// Blocks exports the block database
func (m *DB) Blocks() database.Getter {
	m.blockMutex.RLock()
	defer m.blockMutex.RUnlock()
	return NewBlockFetcherDB(m)
}

// Transactions exports the transactions DB
func (m *DB) Transactions() database.Getter {
	return m.transactions
}

// InputVector exports the inputvector DB
func (m *DB) InputVector() database.Getter {
	return m.inputVector
}

// ErrAlreadyExist error returned when adding an existing value to the database
var ErrAlreadyExist = errors.New("block already exists in database")

// AddBlock adds a block to the database
func (m *DB) AddBlock(bl *types.Block) error {
	m.blockMutex.Lock()
	defer m.blockMutex.Unlock()
	if _, err := m.getBlockBytes(bl.ID()); err == nil {
		m.With().Warning(ErrAlreadyExist.Error(), bl.ID())
		return ErrAlreadyExist
	}
	if err := m.writeBlock(bl); err != nil {
		return err
	}
	return nil
}

// GetBlock gets a block from the database by id
func (m *DB) GetBlock(id types.BlockID) (*types.Block, error) {
	if blkh := m.blockCache.Get(id); blkh != nil {
		return blkh, nil
	}

	b, err := m.getBlockBytes(id)
	if err != nil {
		return nil, err
	}
	mbk := &types.DBBlock{}
	if err := mbk.UnmarshalSSZ(b); err != nil {
		return nil, err
	}
	return mbk.ToBlock(), nil
}

// LayerBlocks retrieves all blocks from a layer by layer index
func (m *DB) LayerBlocks(index types.LayerID) ([]*types.Block, error) {
	ids, err := m.LayerBlockIds(index)
	if err != nil {
		return nil, err
	}

	blocks := make([]*types.Block, 0, len(ids))
	for _, k := range ids {
		block, err := m.GetBlock(k)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve block %s %s", k.String(), err)
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// ForBlockInView traverses all blocks in a view and uses blockHandler func on each block
// The block handler func should return two values - a bool indicating whether or not we should stop traversing after the current block (happy flow)
// and an error indicating that an error occurred while handling the block, the traversing will stop in that case as well (error flow)
func (m *DB) ForBlockInView(view map[types.BlockID]struct{}, layer types.LayerID, blockHandler func(block *types.Block) (bool, error)) error {
	blocksToVisit := list.New()
	for id := range view {
		blocksToVisit.PushBack(id)
	}
	seenBlocks := make(map[types.BlockID]struct{})
	for blocksToVisit.Len() > 0 {
		block, err := m.GetBlock(blocksToVisit.Remove(blocksToVisit.Front()).(types.BlockID))
		if err != nil {
			return err
		}

		// catch blocks that were referenced after more than one layer, and slipped through the stop condition
		if block.LayerIndex.Before(layer) {
			continue
		}

		// execute handler
		stop, err := blockHandler(block)
		if err != nil {
			return err
		}

		if stop {
			m.Log.With().Debug("ForBlockInView stopped", block.ID())
			break
		}

		// stop condition: referenced blocks must be in lower layers, so we don't traverse them
		if block.LayerIndex == layer {
			continue
		}

		// push children to bfs queue
		for _, id := range append(block.ForDiff, append(block.AgainstDiff, block.NeutralDiff...)...) {
			if _, found := seenBlocks[id]; !found {
				seenBlocks[id] = struct{}{}
				blocksToVisit.PushBack(id)
			}
		}
	}
	return nil
}

// LayerBlockIds retrieves all block ids from a layer by layer index
func (m *DB) LayerBlockIds(index types.LayerID) ([]types.BlockID, error) {
	layerBuf := index.Bytes()
	zero := false
	ids := []types.BlockID{}
	it := m.layers.Find(layerBuf)
	// TODO(dshulyak) iterator must be able to return an error
	for it.Next() {
		if len(it.Key()) == len(layerBuf) {
			zero = true
			continue
		}
		var id types.BlockID
		copy(id[:], it.Key()[len(layerBuf):])
		ids = append(ids, id)
	}
	if zero || len(ids) > 0 {
		return ids, nil
	}
	return nil, database.ErrNotFound
}

// EmptyLayerHash is the layer hash for an empty layer
var EmptyLayerHash = types.Hash32{}

// AddZeroBlockLayer tags lyr as a layer without blocks
func (m *DB) AddZeroBlockLayer(index types.LayerID) error {
	err := m.layers.Put(index.Bytes(), nil)
	if err == nil {
		m.persistLayerHash(index, EmptyLayerHash)
	}
	return err
}

func (m *DB) getBlockBytes(id types.BlockID) ([]byte, error) {
	// FIXME(dshulyak) key should be prefixed otherwise collisions are possible
	return m.blocks.Get(id.Bytes())
}

// ContextualValidity retrieves opinion on block from the database
func (m *DB) ContextualValidity(id types.BlockID) (bool, error) {
	b, err := m.contextualValidity.Get(id.Bytes())
	if err != nil {
		return false, err
	}
	return b[0] == 1, nil // bytes to bool
}

// SaveContextualValidity persists opinion on block to the database
func (m *DB) SaveContextualValidity(id types.BlockID, valid bool) error {
	var v []byte
	if valid {
		v = constTrue
	} else {
		v = constFalse
	}
	m.Debug("save contextual validity %v %v", id, valid)
	return m.contextualValidity.Put(id.Bytes(), v)
}

// RecordCoinflip saves the weak coinflip result to memory for the given layer
func (m *DB) RecordCoinflip(ctx context.Context, layerID types.LayerID, coinflip bool) {
	m.WithContext(ctx).With().Info("recording coinflip result for layer in mesh",
		layerID,
		log.Bool("coinflip", coinflip))
	m.coinflips[layerID] = coinflip
}

// GetCoinflip returns the weak coinflip result for the given layer
func (m *DB) GetCoinflip(ctx context.Context, layerID types.LayerID) (bool, bool) {
	coin, exists := m.coinflips[layerID]
	return coin, exists
}

// GetLayerInputVector gets the input vote vector for a layer (hare results)
func (m *DB) GetLayerInputVector(hash types.Hash32) ([]types.BlockID, error) {
	it := m.inputVector.Find(hash.Bytes())
	var ids []types.BlockID
	for it.Next() {
		var id types.BlockID
		copy(id[:], it.Key()[len(hash.Bytes()):])
		ids = append(ids, id)
	}
	return ids, nil
}

// GetLayerInputVectorByID gets the input vote vector for a layer (hare results)
func (m *DB) GetLayerInputVectorByID(id types.LayerID) ([]types.BlockID, error) {
	if m.InputVectorBackupFunc != nil {
		return m.InputVectorBackupFunc(id)
	}
	return m.GetLayerInputVector(types.CalcHash32(id.Bytes()))
}

// SaveLayerInputVectorByID gets the input vote vector for a layer (hare results)
func (m *DB) SaveLayerInputVectorByID(ctx context.Context, id types.LayerID, blks []types.BlockID) error {
	hash := types.CalcHash32(id.Bytes())
	m.WithContext(ctx).With().Info("saving input vector",
		id,
		log.String("iv_hash", hash.ShortString()))
	batch := m.inputVector.NewBatch()
	for _, id := range blks {
		if err := batch.Put(append(hash.Bytes(), id.Bytes()...), nil); err != nil {
			return err
		}
	}
	return batch.Write()
}

func (m *DB) persistProcessedLayer(lyr *ProcessedLayer) error {
	data, err := types.InterfaceToBytes(lyr)
	if err != nil {
		return err
	}
	if err := m.general.Put(constPROCESSED, data); err != nil {
		return err
	}
	m.With().Debug("persisted processed layer",
		lyr.ID,
		log.String("layer_hash", lyr.Hash.ShortString()))
	return nil
}

func (m *DB) recoverProcessedLayer() (*ProcessedLayer, error) {
	processed, err := m.general.Get(constPROCESSED)
	if err != nil {
		return nil, err
	}
	var data ProcessedLayer
	err = types.BytesToInterface(processed, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (m *DB) writeBlock(bl *types.Block) error {
	block := &types.DBBlock{
		MiniBlock: bl.MiniBlock,
		ID:        bl.ID(),
		Signature: bl.Signature,
		MinerID:   bl.MinerID().Bytes(),
	}
	if bytes, err := block.MarshalSSZ(); err != nil {
		return fmt.Errorf("could not encode block: %w", err)
	} else if err := m.blocks.Put(bl.ID().Bytes(), bytes); err != nil {
		return fmt.Errorf("could not add block %v to database: %w", bl.ID(), err)
	} else if err := m.updateLayerWithBlock(bl); err != nil {
		return fmt.Errorf("could not update layer %v with new block %v: %w", bl.Layer(), bl.ID(), err)
	}

	m.blockCache.put(bl)
	return nil
}

func (m *DB) updateLayerWithBlock(blk *types.Block) error {
	var b bytes.Buffer
	b.Write(blk.LayerIndex.Bytes())
	b.Write(blk.ID().Bytes())
	return m.layers.Put(b.Bytes(), nil)
}

func (m *DB) recoverLayerHash(layerID types.LayerID) (types.Hash32, error) {
	h := EmptyLayerHash
	bts, err := m.general.Get(getLayerHashKey(layerID))
	if err != nil {
		return EmptyLayerHash, err
	}
	h.SetBytes(bts)
	return h, nil
}

func (m *DB) persistLayerHash(layerID types.LayerID, hash types.Hash32) {
	if err := m.general.Put(getLayerHashKey(layerID), hash.Bytes()); err != nil {
		m.With().Error("failed to persist layer hash", log.Err(err), layerID,
			log.String("layer_hash", hash.ShortString()))
	}

	// we store a double index here because most of the code uses layer ID as key, currently only sync reads layer by hash
	// when this changes we can simply point to the layes
	if err := m.general.Put(hash.Bytes(), layerID.Bytes()); err != nil {
		m.With().Error("failed to persist layer hash", log.Err(err), layerID,
			log.String("layer_hash", hash.ShortString()))
	}
}

func getRewardKey(l types.LayerID, account types.Address, smesherID types.NodeID) []byte {
	return new(keyBuilder).
		WithAccountRewardsPrefix().
		WithAddress(account).
		WithNodeID(smesherID).
		WithLayerID(l).
		Bytes()
}

func getRewardKeyPrefix(account types.Address) []byte {
	return new(keyBuilder).
		WithAccountRewardsPrefix().
		WithAddress(account).
		Bytes()
}

// This function gets the reward key for a particular smesherID
// format for the index "s_<smesherid>_<accountid>_<layerid> -> r_<accountid>_<smesherid>_<layerid> -> the actual reward"
func getSmesherRewardKey(l types.LayerID, smesherID types.NodeID, account types.Address) []byte {
	return new(keyBuilder).
		WithSmesherRewardsPrefix().
		WithNodeID(smesherID).
		WithAddress(account).
		WithLayerID(l).
		Bytes()
}

// use r_ for one and s_ for the other so the namespaces can't collide
func getSmesherRewardKeyPrefix(smesherID types.NodeID) []byte {
	return new(keyBuilder).
		WithSmesherRewardsPrefix().
		WithNodeID(smesherID).Bytes()
}

func getLayerHashKey(layerID types.LayerID) []byte {
	return new(keyBuilder).
		WithLayerHashPrefix().
		WithLayerID(layerID).Bytes()
}

type keyBuilder struct {
	buf bytes.Buffer
}

func parseLayerIDFromRewardsKey(buf []byte) types.LayerID {
	if len(buf) < 4 {
		panic("key that contains layer id must be atleast 4 bytes")
	}
	return types.NewLayerID(binary.BigEndian.Uint32(buf[len(buf)-4:]))
}

func (k *keyBuilder) WithLayerID(l types.LayerID) *keyBuilder {
	buf := make([]byte, 4)
	// NOTE(dshulyak) big endian produces lexicographically ordered bytes.
	// some queries can be optimizaed by using range queries instead of point queries.
	binary.BigEndian.PutUint32(buf, l.Uint32())
	k.buf.Write(buf)
	return k
}

func (k *keyBuilder) WithOriginPrefix() *keyBuilder {
	k.buf.WriteString("to")
	return k
}

func (k *keyBuilder) WithDestPrefix() *keyBuilder {
	k.buf.WriteString("td")
	return k
}

func (k *keyBuilder) WithLayerHashPrefix() *keyBuilder {
	k.buf.WriteString("lh")
	return k
}

func (k *keyBuilder) WithAccountRewardsPrefix() *keyBuilder {
	k.buf.WriteString("ar")
	return k
}

func (k *keyBuilder) WithSmesherRewardsPrefix() *keyBuilder {
	k.buf.WriteString("sr")
	return k
}

func (k *keyBuilder) WithAddress(addr types.Address) *keyBuilder {
	k.buf.Write(addr.Bytes())
	return k
}

func (k *keyBuilder) WithNodeID(id types.NodeID) *keyBuilder {
	k.buf.WriteString(id.Key)
	k.buf.Write(id.VRFPublicKey)
	return k
}

func (k *keyBuilder) WithBytes(buf []byte) *keyBuilder {
	k.buf.Write(buf)
	return k
}

func (k *keyBuilder) Bytes() []byte {
	return k.buf.Bytes()
}

func getTransactionOriginKey(l types.LayerID, t *types.Transaction) []byte {
	return new(keyBuilder).
		WithOriginPrefix().
		WithBytes(t.Origin().Bytes()).
		WithLayerID(l).
		WithBytes(t.ID().Bytes()).
		Bytes()
}

func getTransactionDestKey(l types.LayerID, t *types.Transaction) []byte {
	return new(keyBuilder).
		WithDestPrefix().
		WithBytes(t.Recipient.Bytes()).
		WithLayerID(l).
		WithBytes(t.ID().Bytes()).
		Bytes()
}

func getTransactionOriginKeyPrefix(l types.LayerID, account types.Address) []byte {
	return new(keyBuilder).
		WithOriginPrefix().
		WithBytes(account.Bytes()).
		WithLayerID(l).
		Bytes()
}

func getTransactionDestKeyPrefix(l types.LayerID, account types.Address) []byte {
	return new(keyBuilder).
		WithDestPrefix().
		WithBytes(account.Bytes()).
		WithLayerID(l).
		Bytes()
}

// DbTransaction is the transaction type stored in DB
type DbTransaction struct {
	*types.Transaction
	Origin  types.Address
	BlockID types.BlockID
	LayerID types.LayerID
}

func newDbTransaction(tx *types.Transaction, blockID types.BlockID, layerID types.LayerID) *DbTransaction {
	return &DbTransaction{
		Transaction: tx,
		Origin:      tx.Origin(),
		BlockID:     blockID,
		LayerID:     layerID,
	}
}

func (t DbTransaction) getTransaction() *types.MeshTransaction {
	t.Transaction.SetOrigin(t.Origin)

	return &types.MeshTransaction{
		Transaction: *t.Transaction,
		LayerID:     t.LayerID,
		BlockID:     t.BlockID,
	}
}

// writeTransactions writes all transactions associated with a block atomically.
func (m *DB) writeTransactions(block *types.Block, txs ...*types.Transaction) error {
	batch := m.transactions.NewBatch()
	for _, t := range txs {
		bytes, err := types.InterfaceToBytes(newDbTransaction(t, block.ID(), block.Layer()))
		if err != nil {
			return fmt.Errorf("could not marshall tx %v to bytes: %v", t.ID().ShortString(), err)
		}
		m.Log.Info("storing tx id %v size %v", t.ID().ShortString(), len(bytes))
		if err := batch.Put(t.ID().Bytes(), bytes); err != nil {
			return fmt.Errorf("could not write tx %v to database: %v", t.ID().ShortString(), err)
		}
		// write extra index for querying txs by account
		if err := batch.Put(getTransactionOriginKey(block.Layer(), t), t.ID().Bytes()); err != nil {
			return fmt.Errorf("could not write tx %v to database: %v", t.ID().ShortString(), err)
		}
		if err := batch.Put(getTransactionDestKey(block.Layer(), t), t.ID().Bytes()); err != nil {
			return fmt.Errorf("could not write tx %v to database: %v", t.ID().ShortString(), err)
		}
		m.Debug("wrote tx %v to db", t.ID().ShortString())
	}
	err := batch.Write()
	if err != nil {
		return fmt.Errorf("failed to write transactions: %v", err)
	}
	return nil
}

// We're not using the existing reward type because the layer is implicit in the key
type dbReward struct {
	TotalReward         uint64
	LayerRewardEstimate uint64
	SmesherID           types.NodeID
	Coinbase            types.Address
	// TotalReward - LayerRewardEstimate = FeesEstimate
}

func (m *DB) writeTransactionRewards(l types.LayerID, accountBlockCount map[types.Address]map[string]uint64, totalReward, layerReward *big.Int) error {
	batch := m.transactions.NewBatch()
	for account, smesherAccountEntry := range accountBlockCount {
		for smesherString, cnt := range smesherAccountEntry {
			smesherEntry, err := types.StringToNodeID(smesherString)
			if err != nil {
				return fmt.Errorf("could not convert String to NodeID for %v: %v", smesherString, err)
			}
			reward := dbReward{TotalReward: cnt * totalReward.Uint64(), LayerRewardEstimate: cnt * layerReward.Uint64(), SmesherID: *smesherEntry, Coinbase: account}
			if b, err := types.InterfaceToBytes(&reward); err != nil {
				return fmt.Errorf("could not marshal reward for %v: %v", account.Short(), err)
			} else if err := batch.Put(getRewardKey(l, account, *smesherEntry), b); err != nil {
				return fmt.Errorf("could not write reward to %v to database: %v", account.Short(), err)
			} else if err := batch.Put(getSmesherRewardKey(l, *smesherEntry, account), getRewardKey(l, account, *smesherEntry)); err != nil {
				return fmt.Errorf("could not write reward key for smesherID %v to database: %v", smesherEntry.ShortString(), err)
			}
		}
	}
	return batch.Write()
}

// GetRewards retrieves account's rewards by address
func (m *DB) GetRewards(account types.Address) (rewards []types.Reward, err error) {
	it := m.transactions.Find(getRewardKeyPrefix(account))
	for it.Next() {
		if it.Key() == nil {
			break
		}
		layer := parseLayerIDFromRewardsKey(it.Key())
		var reward dbReward
		err = types.BytesToInterface(it.Value(), &reward)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal reward: %v", err)
		}
		rewards = append(rewards, types.Reward{
			Layer:               layer,
			TotalReward:         reward.TotalReward,
			LayerRewardEstimate: reward.LayerRewardEstimate,
			SmesherID:           reward.SmesherID,
			Coinbase:            reward.Coinbase,
		})
	}
	return
}

// GetRewardsBySmesherID retrieves rewards by smesherID
func (m *DB) GetRewardsBySmesherID(smesherID types.NodeID) (rewards []types.Reward, err error) {
	it := m.transactions.Find(getSmesherRewardKeyPrefix(smesherID))
	for it.Next() {
		if it.Key() == nil {
			break
		}
		layer := parseLayerIDFromRewardsKey(it.Key())
		if err != nil {
			return nil, fmt.Errorf("error parsing db key %s: %v", it.Key(), err)
		}
		// find the key to the actual reward struct, which is in it.Value()
		var reward dbReward
		rewardBytes, err := m.transactions.Get(it.Value())
		if err != nil {
			return nil, fmt.Errorf("wrong key in db %s: %v", it.Value(), err)
		}
		if err = types.BytesToInterface(rewardBytes, &reward); err != nil {
			return nil, fmt.Errorf("failed to unmarshal reward: %v", err)
		}
		rewards = append(rewards, types.Reward{
			Layer:               layer,
			TotalReward:         reward.TotalReward,
			LayerRewardEstimate: reward.LayerRewardEstimate,
			SmesherID:           reward.SmesherID,
			Coinbase:            reward.Coinbase,
		})
	}
	return
}

func (m *DB) addToUnappliedTxs(txs []*types.Transaction, layer types.LayerID) error {
	groupedTxs := groupByOrigin(txs)

	for addr, accountTxs := range groupedTxs {
		if err := m.addToAccountTxs(addr, accountTxs, layer); err != nil {
			return err
		}
	}
	return nil
}

func (m *DB) addToAccountTxs(addr types.Address, accountTxs []*types.Transaction, layer types.LayerID) error {
	m.unappliedTxsMutex.Lock()
	defer m.unappliedTxsMutex.Unlock()

	// TODO: instead of storing a list, use LevelDB's prefixed keys and then iterate all relevant keys
	pending, err := m.getAccountPendingTxs(addr)
	if err != nil {
		return err
	}
	pending.Add(layer, accountTxs...)
	if err := m.storeAccountPendingTxs(addr, pending); err != nil {
		return err
	}
	return nil
}

func (m *DB) removeFromUnappliedTxs(accepted []*types.Transaction) (grouped map[types.Address][]*types.Transaction, accounts map[types.Address]struct{}) {
	grouped = groupByOrigin(accepted)
	accounts = make(map[types.Address]struct{})
	for account := range grouped {
		accounts[account] = struct{}{}
	}
	for account := range accounts {
		m.removeFromAccountTxs(account, grouped)
	}
	return
}

func (m *DB) removeFromAccountTxs(account types.Address, gAccepted map[types.Address][]*types.Transaction) {
	m.unappliedTxsMutex.Lock()
	defer m.unappliedTxsMutex.Unlock()

	// TODO: instead of storing a list, use LevelDB's prefixed keys and then iterate all relevant keys
	pending, err := m.getAccountPendingTxs(account)
	if err != nil {
		m.With().Error("failed to get account pending txs",
			log.String("address", account.Short()), log.Err(err))
		return
	}
	pending.RemoveAccepted(gAccepted[account])
	if err := m.storeAccountPendingTxs(account, pending); err != nil {
		m.With().Error("failed to store account pending txs",
			log.String("address", account.Short()), log.Err(err))
	}
}

func (m *DB) removeRejectedFromAccountTxs(account types.Address, rejected map[types.Address][]*types.Transaction, layer types.LayerID) {
	m.unappliedTxsMutex.Lock()
	defer m.unappliedTxsMutex.Unlock()

	// TODO: instead of storing a list, use LevelDB's prefixed keys and then iterate all relevant keys
	pending, err := m.getAccountPendingTxs(account)
	if err != nil {
		m.With().Error("failed to get account pending txs",
			layer, log.String("address", account.Short()), log.Err(err))
		return
	}
	pending.RemoveRejected(rejected[account], layer)
	if err := m.storeAccountPendingTxs(account, pending); err != nil {
		m.With().Error("failed to store account pending txs",
			layer, log.String("address", account.Short()), log.Err(err))
	}
}

func (m *DB) storeAccountPendingTxs(account types.Address, pending *pendingtxs.AccountPendingTxs) error {
	if pending.IsEmpty() {
		if err := m.unappliedTxs.Delete(account.Bytes()); err != nil {
			return fmt.Errorf("failed to delete empty pending txs for account %v: %v", account.Short(), err)
		}
		return nil
	}
	if accountTxsBytes, err := types.InterfaceToBytes(&pending); err != nil {
		return fmt.Errorf("failed to marshal account pending txs: %v", err)
	} else if err := m.unappliedTxs.Put(account.Bytes(), accountTxsBytes); err != nil {
		return fmt.Errorf("failed to store mesh txs for address %s", account.Short())
	}
	return nil
}

func (m *DB) getAccountPendingTxs(addr types.Address) (*pendingtxs.AccountPendingTxs, error) {
	accountTxsBytes, err := m.unappliedTxs.Get(addr.Bytes())
	if err != nil && err != database.ErrNotFound {
		return nil, fmt.Errorf("failed to get mesh txs for account %s", addr.Short())
	}
	if err == database.ErrNotFound {
		return pendingtxs.NewAccountPendingTxs(), nil
	}
	var pending pendingtxs.AccountPendingTxs
	if err := types.BytesToInterface(accountTxsBytes, &pending); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account pending txs: %v", err)
	}
	return &pending, nil
}

func groupByOrigin(txs []*types.Transaction) map[types.Address][]*types.Transaction {
	grouped := make(map[types.Address][]*types.Transaction)
	for _, tx := range txs {
		grouped[tx.Origin()] = append(grouped[tx.Origin()], tx)
	}
	return grouped
}

// GetProjection returns projection of address
func (m *DB) GetProjection(addr types.Address, prevNonce, prevBalance uint64) (nonce, balance uint64, err error) {
	pending, err := m.getAccountPendingTxs(addr)
	if err != nil {
		return 0, 0, err
	}
	nonce, balance = pending.GetProjection(prevNonce, prevBalance)
	return nonce, balance, nil
}

// GetTransactions retrieves a list of txs by their id's
func (m *DB) GetTransactions(transactions []types.TransactionID) (txs []*types.Transaction, missing map[types.TransactionID]struct{}) {
	missing = make(map[types.TransactionID]struct{})
	txs = make([]*types.Transaction, 0, len(transactions))
	for _, id := range transactions {
		if tx, err := m.GetMeshTransaction(id); err != nil {
			m.With().Warning("could not fetch tx", id, log.Err(err))
			missing[id] = struct{}{}
		} else {
			txs = append(txs, &tx.Transaction)
		}
	}
	return
}

// GetMeshTransactions retrieves list of txs with information in what blocks and layers they are included.
func (m *DB) GetMeshTransactions(transactions []types.TransactionID) ([]*types.MeshTransaction, map[types.TransactionID]struct{}) {
	var (
		missing = map[types.TransactionID]struct{}{}
		txs     = make([]*types.MeshTransaction, 0, len(transactions))
	)
	for _, id := range transactions {
		tx, err := m.GetMeshTransaction(id)
		if err != nil {
			m.With().Warning("could not fetch tx", id, log.Err(err))
			missing[id] = struct{}{}
		} else {
			txs = append(txs, tx)
		}
	}
	return txs, missing
}

// GetMeshTransaction retrieves a tx by its id
func (m *DB) GetMeshTransaction(id types.TransactionID) (*types.MeshTransaction, error) {
	tBytes, err := m.transactions.Get(id[:])
	if err != nil {
		return nil, fmt.Errorf("could not find transaction in database %v err=%v", hex.EncodeToString(id[:]), err)
	}
	var dbTx DbTransaction
	err = types.BytesToInterface(tBytes, &dbTx)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %v", err)
	}
	return dbTx.getTransaction(), nil
}

// GetTransactionsByDestination retrieves txs by destination and layer
func (m *DB) GetTransactionsByDestination(l types.LayerID, account types.Address) (txs []types.TransactionID) {
	it := m.transactions.Find(getTransactionDestKeyPrefix(l, account))
	for it.Next() {
		if it.Key() == nil {
			break
		}
		var id types.TransactionID
		copy(id[:], it.Value())
		txs = append(txs, id)
	}
	return
}

// GetTransactionsByOrigin retrieves txs by origin and layer
func (m *DB) GetTransactionsByOrigin(l types.LayerID, account types.Address) (txs []types.TransactionID) {
	it := m.transactions.Find(getTransactionOriginKeyPrefix(l, account))
	for it.Next() {
		if it.Key() == nil {
			break
		}
		var id types.TransactionID
		copy(id[:], it.Value())
		txs = append(txs, id)
	}
	return
}

// BlocksByValidity classifies a slice of blocks by validity
func (m *DB) BlocksByValidity(blocks []*types.Block) (validBlocks, invalidBlocks []*types.Block) {
	for _, b := range blocks {
		valid, err := m.ContextualValidity(b.ID())
		if err != nil {
			m.With().Warning("could not get contextual validity by block", b.ID(), log.Err(err))
		}
		if valid {
			validBlocks = append(validBlocks, b)
		} else {
			invalidBlocks = append(invalidBlocks, b)
		}
	}
	return validBlocks, invalidBlocks
}

// LayerContextuallyValidBlocks returns the set of contextually valid block IDs for the provided layer
func (m *DB) LayerContextuallyValidBlocks(layer types.LayerID) (map[types.BlockID]struct{}, error) {
	if layer == types.NewLayerID(0) || layer == types.NewLayerID(1) {
		v, err := m.LayerBlockIds(layer)
		if err != nil {
			m.With().Error("could not get layer block ids", layer, log.Err(err))
			return nil, err
		}

		mp := make(map[types.BlockID]struct{}, len(v))
		for _, blk := range v {
			mp[blk] = struct{}{}
		}

		return mp, nil
	}

	blockIds, err := m.LayerBlockIds(layer)
	if err != nil {
		return nil, err
	}

	validBlks := make(map[types.BlockID]struct{})

	cvErrors := make(map[string][]types.BlockID)
	cvErrorCount := 0
	for _, b := range blockIds {
		valid, err := m.ContextualValidity(b)
		if err != nil {
			cvErrors[err.Error()] = append(cvErrors[err.Error()], b)
			cvErrorCount++
		}

		if !valid {
			continue
		}

		validBlks[b] = struct{}{}
	}

	m.With().Info("count of contextually valid blocks in layer",
		layer,
		log.Int("count_valid", len(validBlks)),
		log.Int("count_error", cvErrorCount),
		log.Int("count_total", len(blockIds)))
	if cvErrorCount != 0 {
		m.With().Error("errors occurred getting contextual validity", layer,
			log.String("errors", fmt.Sprint(cvErrors)))
	}
	return validBlks, nil
}

// Persist persists an item v into the database using key as its id
func (m *DB) Persist(key []byte, v interface{}) error {
	buf, err := types.InterfaceToBytes(v)
	if err != nil {
		panic(err)
	}
	return m.general.Put(key, buf)
}

// Retrieve retrieves item by key into v
func (m *DB) Retrieve(key []byte, v interface{}) (interface{}, error) {
	val, err := m.general.Get(key)
	if err != nil {
		m.Warning("failed retrieving object from db ", err)
		return nil, err
	}

	if val == nil {
		return nil, fmt.Errorf("no such value in database db ")
	}

	if err := types.BytesToInterface(val, v); err != nil {
		return nil, fmt.Errorf("failed decoding object from db %v", err)
	}

	return v, nil
}

func (m *DB) cacheWarmUpFromTo(from types.LayerID, to types.LayerID) error {
	m.Info("warming up cache with layers %v to %v", from, to)
	for i := from; i.Before(to); i = i.Add(1) {

		select {
		case <-m.exit:
			m.Info("shutdown during cache warm up")
			return nil
		default:
		}

		layer, err := m.LayerBlockIds(i)
		if err != nil {
			return fmt.Errorf("could not get layer %v from database %v", layer, err)
		}

		for _, b := range layer {
			block, blockErr := m.GetBlock(b)
			if blockErr != nil {
				return fmt.Errorf("could not get bl %v from database %v", b, blockErr)
			}
			m.blockCache.put(block)
		}

	}
	m.Info("done warming up cache")

	return nil
}

// NewBlockFetcherDB returns reference to a BlockFetcherDB instance.
func NewBlockFetcherDB(mdb *DB) *BlockFetcherDB {
	return &BlockFetcherDB{mdb: mdb}
}

// BlockFetcherDB implements API that allows fetcher to get a block from a remote database.
type BlockFetcherDB struct {
	mdb *DB
}

// Get types.Block encoded in byte using hash.
func (db *BlockFetcherDB) Get(hash []byte) ([]byte, error) {
	id := types.BlockID(types.BytesToHash(hash).ToHash20())
	blk, err := db.mdb.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return types.InterfaceToBytes(blk)
}
