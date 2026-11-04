package app

import (
	"encoding/json"
	"os"
	"testing"

	db "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"

	"cosmossdk.io/math"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

var emptyWasmOpts []wasm.Option

func setBech32ForTest() {
	// SDK config
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(Bech32PrefixAccAddr, Bech32PrefixAccPub)
	cfg.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
	cfg.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)
	cfg.SetAddressVerifier(wasmtypes.VerifyAddressLen())
}

func TestWasmdExport(t *testing.T) {
	setBech32ForTest()

	db := db.NewMemDB()
	gapp := NewJackalApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, DefaultNodeHome, 0, MakeEncodingConfig(), EmptyBaseAppOptions{}, emptyWasmOpts)

	genesisState := NewDefaultGenesisState()
	// WO-75 test scaffolding: seed one bonded dummy validator (test genesis only),
	// reusing the WO-69 pattern (c707d002) -> v0.47 panics on an empty validator
	// set after InitGenesis.
	bondAmt := sdk.TokensFromConsensusPower(100, sdk.DefaultPowerReduction)
	privKey := ed25519.GenPrivKeyFromSecret([]byte("wo75-dummy-validator"))
	pubKeyAny, err := codectypes.NewAnyWithValue(privKey.PubKey())
	if err != nil {
		panic(err)
	}
	appCodec := gapp.AppCodec()

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

	var bankGen banktypes.GenesisState
	appCodec.MustUnmarshalJSON(genesisState[banktypes.ModuleName], &bankGen)
	bankGen.Balances = append(bankGen.Balances, banktypes.Balance{
		Address: authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(bondDenom, bondAmt)),
	})
	bankGen.Balances = banktypes.SanitizeGenesisBalances(bankGen.Balances)
	bankGen.Supply = bankGen.Supply.Add(sdk.NewCoin(bondDenom, bondAmt))
	genesisState[banktypes.ModuleName] = appCodec.MustMarshalJSON(&bankGen)
	stateBytes, err := json.MarshalIndent(genesisState, "", "  ")
	require.NoError(t, err)

	// Initialize the chain
	gapp.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)
	gapp.Commit()

	// Making a new app object with the db, so that initchain hasn't been called
	newGapp := NewJackalApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, DefaultNodeHome, 0, MakeEncodingConfig(), EmptyBaseAppOptions{}, emptyWasmOpts)
	_, err = newGapp.ExportAppStateAndValidators(false, []string{}, []string{})
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

// ensure that blocked addresses are properly set in bank keeper
func TestBlockedAddrs(t *testing.T) {
	setBech32ForTest()

	db := db.NewMemDB()
	gapp := NewJackalApp(log.NewTMLogger(log.NewSyncWriter(os.Stdout)), db, nil, true, map[int64]bool{}, DefaultNodeHome, 0, MakeEncodingConfig(), EmptyBaseAppOptions{}, emptyWasmOpts)

	for acc := range maccPerms {
		t.Run(acc, func(t *testing.T) {
			require.True(t, gapp.BankKeeper.BlockedAddr(gapp.AccountKeeper.GetModuleAddress(acc)),
				"ensure that blocked addresses are properly set in bank keeper",
			)
		})
	}
}

func TestGetMaccPerms(t *testing.T) {
	dup := GetMaccPerms()
	require.Equal(t, maccPerms, dup, "duplicated module account permissions differed from actual module account permissions")
}

func setGenesis(gapp *JackalApp) error { //nolint:unused
	genesisState := NewDefaultGenesisState()
	stateBytes, err := json.MarshalIndent(genesisState, "", " ")
	if err != nil {
		return err
	}

	// Initialize the chain
	gapp.InitChain(
		abci.RequestInitChain{
			Validators:    []abci.ValidatorUpdate{},
			AppStateBytes: stateBytes,
		},
	)

	gapp.Commit()
	return nil
}
