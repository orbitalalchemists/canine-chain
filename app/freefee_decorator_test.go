package app

import (
	"encoding/json"
	"testing"

	"cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	feegrant "github.com/cosmos/cosmos-sdk/x/feegrant"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	storagetypes "github.com/jackalLabs/canine-chain/v5/x/storage/types"
)

// WO-84 fee/event parity rows for the FreeFeeDecorator ante seam.
// Harness: the real setup() app with WO-75 genesis (one bonded dummy
// validator), direct genesis funding of the test addresses, ledger-style
// assertions. Stage A signature: blueprint 04-results/6.8-testnet-dry-run.md
// (9-row whitelist matrix). Deferred to Stage B-live/Stage C: the
// empty-msg probe (unreachable at signing, v0.45 baseline banked) and
// the handler-failure symmetry row.
//
// Funding note: the house AddTestAddrs helper mints through the jklmint
// module account (name "mint"), which stock v0.45 genesis created and this
// app's InitGenesis does not - dead code on this branch (WO-85 open). The
// harness therefore funds its accounts directly in genesis, no minting.

// feeParityStub is a minimal sdk.FeeTx for the parity rows: the
// decorator reads only msgs, fee, gas, payer and granter. No signing
// machinery anywhere (signature checks are other decorators' work).
type feeParityStub struct {
	msgs    []sdk.Msg
	fee     sdk.Coins
	gas     uint64
	payer   sdk.AccAddress
	granter sdk.AccAddress
}

func (s *feeParityStub) GetMsgs() []sdk.Msg       { return s.msgs }
func (s *feeParityStub) ValidateBasic() error     { return nil }
func (s *feeParityStub) GetGas() uint64           { return s.gas }
func (s *feeParityStub) GetFee() sdk.Coins        { return s.fee }
func (s *feeParityStub) FeePayer() sdk.AccAddress { return s.payer }

func (s *feeParityStub) FeeGranter() sdk.AccAddress { return s.granter }

// newFeeParityHarness boots a real app with WO-75-style genesis (one
// bonded dummy validator), runs InitChain + Commit, and returns the
// app plus a working context at height 1 (check-state base, flipped
// to DeliverTx). Rows add their own knobs (IsCheckTx, MinGasPrices,
// GasMeter) on top of this ctx.
func newFeeParityHarness(t *testing.T) (*JackalApp, sdk.Context) {
	t.Helper()
	setBech32ForTest()

	gapp, genesisState := setup(t, true, 0)
	appCodec := gapp.AppCodec()

	// WO-75 pattern (c707d002): v0.47 panics on an empty validator set
	// after InitGenesis, so the test genesis seeds one bonded validator.
	bondAmt := sdk.TokensFromConsensusPower(100, sdk.DefaultPowerReduction)
	privKey := ed25519.GenPrivKeyFromSecret([]byte("wo84-fee-parity-validator"))
	pubKeyAny, err := codectypes.NewAnyWithValue(privKey.PubKey())
	require.NoError(t, err)

	var stakingGen stakingtypes.GenesisState
	appCodec.MustUnmarshalJSON(genesisState[stakingtypes.ModuleName], &stakingGen)
	bondDenom := stakingGen.Params.BondDenom
	valAddr := sdk.ValAddress(privKey.PubKey().Address()).String()
	stakingGen.Validators = append(stakingGen.Validators, stakingtypes.Validator{
		OperatorAddress: valAddr,
		ConsensusPubkey: pubKeyAny,
		Status:          stakingtypes.Bonded,
		Tokens:          bondAmt,
		DelegatorShares: math.LegacyOneDec(),
	})
	stakingGen.Delegations = append(stakingGen.Delegations, stakingtypes.Delegation{
		DelegatorAddress: sdk.AccAddress(privKey.PubKey().Address()).String(),
		ValidatorAddress: valAddr,
		Shares:           math.LegacyOneDec(),
	})
	genesisState[stakingtypes.ModuleName] = appCodec.MustMarshalJSON(&stakingGen)

	// Direct funding: 3 deterministic addresses (P4 needs granter +
	// two grantees; every other row uses just the first). Same helper
	// and same order as the old AddTestAddrs, so rows stay unchanged
	// apart from the call.
	fundAddrs := createIncrementalAccounts(3)
	fundAmt := sdk.TokensFromConsensusPower(10, sdk.DefaultPowerReduction)

	var authGen authtypes.GenesisState
	appCodec.MustUnmarshalJSON(genesisState[authtypes.ModuleName], &authGen)
	for i, addr := range fundAddrs {
		acc := authtypes.NewBaseAccount(addr, nil, uint64(i)+1, 0)
		accAny, err := codectypes.NewAnyWithValue(acc)
		require.NoError(t, err)
		authGen.Accounts = append(authGen.Accounts, accAny)
	}
	genesisState[authtypes.ModuleName] = appCodec.MustMarshalJSON(&authGen)

	var bankGen banktypes.GenesisState
	appCodec.MustUnmarshalJSON(genesisState[banktypes.ModuleName], &bankGen)
	bankGen.Balances = append(bankGen.Balances, banktypes.Balance{
		Address: authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(bondDenom, bondAmt)),
	})
	for _, addr := range fundAddrs {
		bankGen.Balances = append(bankGen.Balances, banktypes.Balance{
			Address: addr.String(),
			Coins:   sdk.NewCoins(sdk.NewCoin(bondDenom, fundAmt)),
		})
	}
	bankGen.Balances = banktypes.SanitizeGenesisBalances(bankGen.Balances)
	bankGen.Supply = bankGen.Supply.Add(sdk.NewCoin(bondDenom, bondAmt))
	for range fundAddrs {
		bankGen.Supply = bankGen.Supply.Add(sdk.NewCoin(bondDenom, fundAmt))
	}
	genesisState[banktypes.ModuleName] = appCodec.MustMarshalJSON(&bankGen)

	stateBytes, err := json.MarshalIndent(genesisState, "", "  ")
	require.NoError(t, err)

	gapp.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	gapp.Commit()

	// Post-Commit only checkState is alive (NewContext(false) panics on
	// the nil deliverState), so take the check-state base and flip to
	// DeliverTx for the rows' default context.
	ctx := gapp.NewContext(true, tmproto.Header{Height: 1, ChainID: "oa-84"}).WithIsCheckTx(false)
	return gapp, ctx
}

func hasEventType(events sdk.Events, eventType string) bool {
	for _, e := range events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}

// TestFreeFeeP1MinFeeRejection (row P1): a whitelisted tx whose fee is
// below the node's min-gas-prices is rejected with ErrInsufficientFee in
// CheckTx and nothing is charged (fork CheckTx policy, blueprint 6.6).
func TestFreeFeeP1MinFeeRejection(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	payer := createIncrementalAccounts(1)[0]
	bondDenom := gapp.stakingKeeper.BondDenom(ctx)

	// min gas price 0.001 x 100,000 gas => required fee 100; declared fee
	// is empty, so the decorator's min-fee check must fire.
	minGasPrice := sdk.NewDecCoinFromDec(bondDenom, math.LegacyNewDecWithPrec(1, 3))
	stub := &feeParityStub{
		msgs:  []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:   sdk.Coins{},
		gas:   100000,
		payer: payer,
	}

	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))

	checkCtx := ctx.WithIsCheckTx(true).WithMinGasPrices(sdk.DecCoins{minGasPrice})
	feeCollectorAddr := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	payerBefore := gapp.BankKeeper.GetBalance(checkCtx, payer, bondDenom).Amount.Int64()
	collectorBefore := gapp.BankKeeper.GetAllBalances(checkCtx, feeCollectorAddr).AmountOf(bondDenom).Int64()

	_, err := antehandler(checkCtx, stub, false)
	require.ErrorIs(t, err, sdkerrors.ErrInsufficientFee, "P1: below-min fee must be rejected in CheckTx")

	require.Equal(t, payerBefore, gapp.BankKeeper.GetBalance(checkCtx, payer, bondDenom).Amount.Int64(), "P1: payer must not be charged")
	require.Equal(t, collectorBefore, gapp.BankKeeper.GetAllBalances(checkCtx, feeCollectorAddr).AmountOf(bondDenom).Int64(), "P1: fee collector must stay empty")
}

// TestFreeFeeP2WhitelistedNotCharged (row P2): a whitelisted tx with a
// sufficient fee passes CheckTx, passes DeliverTx (the min-fee check is
// CheckTx-only), pays nothing, carries fork priority 0, and still emits
// the declared-fee tx event.
func TestFreeFeeP2WhitelistedNotCharged(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	payer := createIncrementalAccounts(1)[0]
	bondDenom := gapp.stakingKeeper.BondDenom(ctx)

	minGasPrice := sdk.NewDecCoinFromDec(bondDenom, math.LegacyNewDecWithPrec(1, 3))
	fee := sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(200)))
	stub := &feeParityStub{
		msgs:  []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:   fee,
		gas:   100000,
		payer: payer,
	}
	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))
	feeCollectorAddr := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	payerBefore := gapp.BankKeeper.GetBalance(ctx, payer, bondDenom).Amount.Int64()
	collectorBefore := gapp.BankKeeper.GetAllBalances(ctx, feeCollectorAddr).AmountOf(bondDenom).Int64()

	checkCtx := ctx.WithIsCheckTx(true).WithMinGasPrices(sdk.DecCoins{minGasPrice})
	_, err := antehandler(checkCtx, stub, false)
	require.NoError(t, err, "P2: sufficient fee must pass CheckTx")

	deliverCtx := ctx.WithIsCheckTx(false).WithMinGasPrices(sdk.DecCoins{minGasPrice})
	newCtx, err := antehandler(deliverCtx, stub, false)
	require.NoError(t, err, "P2: min-fee check must be CheckTx-only")

	require.Equal(t, int64(0), newCtx.Priority(), "P2: fork priority is hard-zero")
	require.True(t, hasEventType(newCtx.EventManager().Events(), sdk.EventTypeTx), "P2: declared-fee tx event must be emitted")
	require.Equal(t, payerBefore, gapp.BankKeeper.GetBalance(deliverCtx, payer, bondDenom).Amount.Int64(), "P2: payer must not be charged")
	require.Equal(t, collectorBefore, gapp.BankKeeper.GetAllBalances(deliverCtx, feeCollectorAddr).AmountOf(bondDenom).Int64(), "P2: fee collector must stay empty")
}

// TestFreeFeeP3ControlCharged (row P3): a non-whitelisted bank MsgSend
// control with the same fee is charged for real: collector +200, payer
// -200 (ledger deltas, Stage A rows 6/7/8 signature).
func TestFreeFeeP3ControlCharged(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	payer := createIncrementalAccounts(1)[0]
	bondDenom := gapp.stakingKeeper.BondDenom(ctx)

	fee := sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(200)))
	stub := &feeParityStub{
		msgs:  []sdk.Msg{&banktypes.MsgSend{}},
		fee:   fee,
		gas:   100000,
		payer: payer,
	}
	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))
	feeCollectorAddr := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	payerBefore := gapp.BankKeeper.GetBalance(ctx, payer, bondDenom).Amount.Int64()
	collectorBefore := gapp.BankKeeper.GetAllBalances(ctx, feeCollectorAddr).AmountOf(bondDenom).Int64()

	_, err := antehandler(ctx, stub, false)
	require.NoError(t, err, "P3: sufficient-fee control must pass DeliverTx")

	require.Equal(t, collectorBefore+200, gapp.BankKeeper.GetAllBalances(ctx, feeCollectorAddr).AmountOf(bondDenom).Int64(), "P3: collector must receive the fee")
	require.Equal(t, payerBefore-200, gapp.BankKeeper.GetBalance(ctx, payer, bondDenom).Amount.Int64(), "P3: payer must be debited")
}

// TestFreeFeeP4Feegrant (row P4): a whitelisted tx paid via feegrant
// passes, fires use_feegrant through the real keeper, consumes the
// allowance even though it is whitelisted (fork behaviour, Stage A row
// 5), debits nobody, and rejects a fee over the allowance limit with
// the fork-verbatim wrapper text.
func TestFreeFeeP4Feegrant(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	addrs := createIncrementalAccounts(3)
	granter := addrs[0]
	grantee := addrs[1]
	grantee2 := addrs[2]
	bondDenom := gapp.stakingKeeper.BondDenom(ctx)

	fee := sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(200)))
	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))
	granterBefore := gapp.BankKeeper.GetBalance(ctx, granter, bondDenom).Amount.Int64()

	err := gapp.feeGrantKeeper.GrantAllowance(ctx, granter, grantee, &feegrant.BasicAllowance{
		SpendLimit: sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(1000))),
	})
	require.NoError(t, err)

	stub := &feeParityStub{
		msgs:    []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:     fee,
		gas:     100000,
		payer:   grantee,
		granter: granter,
	}
	newCtx, err := antehandler(ctx, stub, false)
	require.NoError(t, err, "P4: whitelisted tx via feegrant must pass")
	require.True(t, hasEventType(newCtx.EventManager().Events(), feegrant.EventTypeUseFeeGrant), "P4: use_feegrant event must fire via the real keeper")

	allow, err := gapp.feeGrantKeeper.GetAllowance(ctx, granter, grantee)
	require.NoError(t, err)
	basic, ok := allow.(*feegrant.BasicAllowance)
	require.True(t, ok, "P4: allowance must remain BasicAllowance")
	require.True(t, basic.SpendLimit.IsEqual(sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(800)))), "P4: allowance must be consumed even for whitelisted txs (fork behaviour)")

	require.Equal(t, granterBefore, gapp.BankKeeper.GetBalance(ctx, granter, bondDenom).Amount.Int64(), "P4: granter must not be debited")
	require.Equal(t, int64(0), gapp.BankKeeper.GetAllBalances(ctx, authtypes.NewModuleAddress(authtypes.FeeCollectorName)).AmountOf(bondDenom).Int64(), "P4: collector must stay empty")

	// Sub-case: fee meets the (unset, hence zero) min-gas-prices but
	// exceeds the allowance limit; the decorator's feegrant wrapper must
	// fire with its fork-verbatim text (freefee_decorator.go line 107).
	err = gapp.feeGrantKeeper.GrantAllowance(ctx, granter, grantee2, &feegrant.BasicAllowance{
		SpendLimit: sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(100))),
	})
	require.NoError(t, err)

	stub2 := &feeParityStub{
		msgs:    []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:     fee,
		gas:     100000,
		payer:   grantee2,
		granter: granter,
	}
	_, err = antehandler(ctx, stub2, false)
	require.ErrorContains(t, err, "not allowed to pay fees from", "P4: fork-verbatim feegrant wrapper must fire")
}

// TestFreeFeeP5SimulateSkips (row P5): simulate mode skips both the
// min-fee check and charging, even in CheckTx against a high
// min-gas-prices (upstream ZeroGas/EnsureMempoolFees simulate idiom).
func TestFreeFeeP5SimulateSkips(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	payer := createIncrementalAccounts(1)[0]
	bondDenom := gapp.stakingKeeper.BondDenom(ctx)

	minGasPrice := sdk.NewDecCoinFromDec(bondDenom, math.LegacyNewDecWithPrec(1, 3))
	stub := &feeParityStub{
		msgs:  []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:   sdk.Coins{},
		gas:   100000,
		payer: payer,
	}
	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))

	simCtx := ctx.WithIsCheckTx(true).WithMinGasPrices(sdk.DecCoins{minGasPrice})
	payerBefore := gapp.BankKeeper.GetBalance(simCtx, payer, bondDenom).Amount.Int64()

	_, err := antehandler(simCtx, stub, true)
	require.NoError(t, err, "P5: simulate must skip the min-fee check")
	require.Equal(t, payerBefore, gapp.BankKeeper.GetBalance(simCtx, payer, bondDenom).Amount.Int64(), "P5: nobody charged in simulate")
}

// TestFreeFeeP6ZeroGas (row P6): zero gas is rejected with
// ErrInvalidGasLimit at height > 0 when not simulating - the v0.47
// stock check v0.45 lacked (blueprint 6.8 Stage B contrast, armed).
func TestFreeFeeP6ZeroGas(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	payer := createIncrementalAccounts(1)[0]

	stub := &feeParityStub{
		msgs:  []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:   sdk.Coins{},
		gas:   0,
		payer: payer,
	}
	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))

	checkCtx := ctx.WithIsCheckTx(true)
	_, err := antehandler(checkCtx, stub, false)
	require.ErrorIs(t, err, sdkerrors.ErrInvalidGasLimit, "P6: zero gas must be rejected at height > 0 (v0.47 stock check)")
}

// TestFreeFeeINVWhitelistGasDelta (row INV): on the same harness, the
// whitelisted run consumes strictly less gas than the charged control
// (the decorator adds no bank-send work). Absolute numbers are Stage
// C's; this row asserts only the relative invariant.
func TestFreeFeeINVWhitelistGasDelta(t *testing.T) {
	gapp, ctx := newFeeParityHarness(t)

	payer := createIncrementalAccounts(1)[0]
	bondDenom := gapp.stakingKeeper.BondDenom(ctx)

	fee := sdk.NewCoins(sdk.NewCoin(bondDenom, math.NewInt(200)))
	antehandler := sdk.ChainAnteDecorators(NewFreeFeeDecorator(gapp.AccountKeeper, gapp.BankKeeper, gapp.feeGrantKeeper))

	wlStub := &feeParityStub{
		msgs:  []sdk.Msg{&storagetypes.MsgAttest{}},
		fee:   fee,
		gas:   100000,
		payer: payer,
	}
	ctrlStub := &feeParityStub{
		msgs:  []sdk.Msg{&banktypes.MsgSend{}},
		fee:   fee,
		gas:   100000,
		payer: payer,
	}

	wlCtx := ctx.WithGasMeter(sdk.NewGasMeter(10000000))
	_, err := antehandler(wlCtx, wlStub, false)
	require.NoError(t, err)

	ctrlCtx := ctx.WithGasMeter(sdk.NewGasMeter(10000000))
	_, err = antehandler(ctrlCtx, ctrlStub, false)
	require.NoError(t, err)

	require.Greater(t, ctrlCtx.GasMeter().GasConsumed(), wlCtx.GasMeter().GasConsumed(), "INV: whitelisted run must consume less gas than the charged control")
}
