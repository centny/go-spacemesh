// Code generated by MockGen. DO NOT EDIT.
// Source: ./tortoisebeacon/tortoise_beacon.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	types "github.com/spacemeshos/go-spacemesh/common/types"
	service "github.com/spacemeshos/go-spacemesh/p2p/service"
	timesync "github.com/spacemeshos/go-spacemesh/timesync"
	weakcoin "github.com/spacemeshos/go-spacemesh/tortoisebeacon/weakcoin"
)

// Mockbroadcaster is a mock of broadcaster interface.
type Mockbroadcaster struct {
	ctrl     *gomock.Controller
	recorder *MockbroadcasterMockRecorder
}

// MockbroadcasterMockRecorder is the mock recorder for Mockbroadcaster.
type MockbroadcasterMockRecorder struct {
	mock *Mockbroadcaster
}

// NewMockbroadcaster creates a new mock instance.
func NewMockbroadcaster(ctrl *gomock.Controller) *Mockbroadcaster {
	mock := &Mockbroadcaster{ctrl: ctrl}
	mock.recorder = &MockbroadcasterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *Mockbroadcaster) EXPECT() *MockbroadcasterMockRecorder {
	return m.recorder
}

// Broadcast mocks base method.
func (m *Mockbroadcaster) Broadcast(ctx context.Context, channel string, data []byte) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Broadcast", ctx, channel, data)
	ret0, _ := ret[0].(error)
	return ret0
}

// Broadcast indicates an expected call of Broadcast.
func (mr *MockbroadcasterMockRecorder) Broadcast(ctx, channel, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Broadcast", reflect.TypeOf((*Mockbroadcaster)(nil).Broadcast), ctx, channel, data)
}

// MocktortoiseBeaconDB is a mock of tortoiseBeaconDB interface.
type MocktortoiseBeaconDB struct {
	ctrl     *gomock.Controller
	recorder *MocktortoiseBeaconDBMockRecorder
}

// MocktortoiseBeaconDBMockRecorder is the mock recorder for MocktortoiseBeaconDB.
type MocktortoiseBeaconDBMockRecorder struct {
	mock *MocktortoiseBeaconDB
}

// NewMocktortoiseBeaconDB creates a new mock instance.
func NewMocktortoiseBeaconDB(ctrl *gomock.Controller) *MocktortoiseBeaconDB {
	mock := &MocktortoiseBeaconDB{ctrl: ctrl}
	mock.recorder = &MocktortoiseBeaconDBMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MocktortoiseBeaconDB) EXPECT() *MocktortoiseBeaconDBMockRecorder {
	return m.recorder
}

// GetTortoiseBeacon mocks base method.
func (m *MocktortoiseBeaconDB) GetTortoiseBeacon(epochID types.EpochID) (types.Hash32, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTortoiseBeacon", epochID)
	ret0, _ := ret[0].(types.Hash32)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTortoiseBeacon indicates an expected call of GetTortoiseBeacon.
func (mr *MocktortoiseBeaconDBMockRecorder) GetTortoiseBeacon(epochID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTortoiseBeacon", reflect.TypeOf((*MocktortoiseBeaconDB)(nil).GetTortoiseBeacon), epochID)
}

// SetTortoiseBeacon mocks base method.
func (m *MocktortoiseBeaconDB) SetTortoiseBeacon(epochID types.EpochID, beacon types.Hash32) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetTortoiseBeacon", epochID, beacon)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetTortoiseBeacon indicates an expected call of SetTortoiseBeacon.
func (mr *MocktortoiseBeaconDBMockRecorder) SetTortoiseBeacon(epochID, beacon interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetTortoiseBeacon", reflect.TypeOf((*MocktortoiseBeaconDB)(nil).SetTortoiseBeacon), epochID, beacon)
}

// Mockcoin is a mock of coin interface.
type Mockcoin struct {
	ctrl     *gomock.Controller
	recorder *MockcoinMockRecorder
}

// MockcoinMockRecorder is the mock recorder for Mockcoin.
type MockcoinMockRecorder struct {
	mock *Mockcoin
}

// NewMockcoin creates a new mock instance.
func NewMockcoin(ctrl *gomock.Controller) *Mockcoin {
	mock := &Mockcoin{ctrl: ctrl}
	mock.recorder = &MockcoinMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *Mockcoin) EXPECT() *MockcoinMockRecorder {
	return m.recorder
}

// FinishEpoch mocks base method.
func (m *Mockcoin) FinishEpoch() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FinishEpoch")
}

// FinishEpoch indicates an expected call of FinishEpoch.
func (mr *MockcoinMockRecorder) FinishEpoch() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FinishEpoch", reflect.TypeOf((*Mockcoin)(nil).FinishEpoch))
}

// FinishRound mocks base method.
func (m *Mockcoin) FinishRound() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FinishRound")
}

// FinishRound indicates an expected call of FinishRound.
func (mr *MockcoinMockRecorder) FinishRound() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FinishRound", reflect.TypeOf((*Mockcoin)(nil).FinishRound))
}

// Get mocks base method.
func (m *Mockcoin) Get(arg0 types.EpochID, arg1 types.RoundID) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", arg0, arg1)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Get indicates an expected call of Get.
func (mr *MockcoinMockRecorder) Get(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*Mockcoin)(nil).Get), arg0, arg1)
}

// HandleSerializedMessage mocks base method.
func (m *Mockcoin) HandleSerializedMessage(arg0 context.Context, arg1 service.GossipMessage, arg2 service.Fetcher) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "HandleSerializedMessage", arg0, arg1, arg2)
}

// HandleSerializedMessage indicates an expected call of HandleSerializedMessage.
func (mr *MockcoinMockRecorder) HandleSerializedMessage(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandleSerializedMessage", reflect.TypeOf((*Mockcoin)(nil).HandleSerializedMessage), arg0, arg1, arg2)
}

// StartEpoch mocks base method.
func (m *Mockcoin) StartEpoch(arg0 types.EpochID, arg1 weakcoin.UnitAllowances) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "StartEpoch", arg0, arg1)
}

// StartEpoch indicates an expected call of StartEpoch.
func (mr *MockcoinMockRecorder) StartEpoch(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StartEpoch", reflect.TypeOf((*Mockcoin)(nil).StartEpoch), arg0, arg1)
}

// StartRound mocks base method.
func (m *Mockcoin) StartRound(arg0 context.Context, arg1 types.RoundID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "StartRound", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// StartRound indicates an expected call of StartRound.
func (mr *MockcoinMockRecorder) StartRound(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "StartRound", reflect.TypeOf((*Mockcoin)(nil).StartRound), arg0, arg1)
}

// MocklayerClock is a mock of layerClock interface.
type MocklayerClock struct {
	ctrl     *gomock.Controller
	recorder *MocklayerClockMockRecorder
}

// MocklayerClockMockRecorder is the mock recorder for MocklayerClock.
type MocklayerClockMockRecorder struct {
	mock *MocklayerClock
}

// NewMocklayerClock creates a new mock instance.
func NewMocklayerClock(ctrl *gomock.Controller) *MocklayerClock {
	mock := &MocklayerClock{ctrl: ctrl}
	mock.recorder = &MocklayerClockMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MocklayerClock) EXPECT() *MocklayerClockMockRecorder {
	return m.recorder
}

// AwaitLayer mocks base method.
func (m *MocklayerClock) AwaitLayer(layerID types.LayerID) chan struct{} {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AwaitLayer", layerID)
	ret0, _ := ret[0].(chan struct{})
	return ret0
}

// AwaitLayer indicates an expected call of AwaitLayer.
func (mr *MocklayerClockMockRecorder) AwaitLayer(layerID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AwaitLayer", reflect.TypeOf((*MocklayerClock)(nil).AwaitLayer), layerID)
}

// GetCurrentLayer mocks base method.
func (m *MocklayerClock) GetCurrentLayer() types.LayerID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCurrentLayer")
	ret0, _ := ret[0].(types.LayerID)
	return ret0
}

// GetCurrentLayer indicates an expected call of GetCurrentLayer.
func (mr *MocklayerClockMockRecorder) GetCurrentLayer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCurrentLayer", reflect.TypeOf((*MocklayerClock)(nil).GetCurrentLayer))
}

// LayerToTime mocks base method.
func (m *MocklayerClock) LayerToTime(id types.LayerID) time.Time {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LayerToTime", id)
	ret0, _ := ret[0].(time.Time)
	return ret0
}

// LayerToTime indicates an expected call of LayerToTime.
func (mr *MocklayerClockMockRecorder) LayerToTime(id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LayerToTime", reflect.TypeOf((*MocklayerClock)(nil).LayerToTime), id)
}

// Subscribe mocks base method.
func (m *MocklayerClock) Subscribe() timesync.LayerTimer {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Subscribe")
	ret0, _ := ret[0].(timesync.LayerTimer)
	return ret0
}

// Subscribe indicates an expected call of Subscribe.
func (mr *MocklayerClockMockRecorder) Subscribe() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Subscribe", reflect.TypeOf((*MocklayerClock)(nil).Subscribe))
}

// Unsubscribe mocks base method.
func (m *MocklayerClock) Unsubscribe(timer timesync.LayerTimer) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Unsubscribe", timer)
}

// Unsubscribe indicates an expected call of Unsubscribe.
func (mr *MocklayerClockMockRecorder) Unsubscribe(timer interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Unsubscribe", reflect.TypeOf((*MocklayerClock)(nil).Unsubscribe), timer)
}
