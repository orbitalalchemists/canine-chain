package keeper_test

import (
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/cometbft/cometbft/libs/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/math"
	dbm "github.com/cometbft/cometbft-db"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/jackalLabs/canine-chain/v5/x/jklmint/types"

	jklapp "github.com/jackalLabs/canine-chain/v5/app"
)

// returns context and an app with updated mint keeper
//
//nolint:unused
func createTestApp(isCheckTx bool) (*jklapp.JackalApp, sdk.Context) {
	app := setup(isCheckTx)

	ctx := app.NewContext(isCheckTx, tmproto.Header{})

	app.MintKeeper.SetParams(ctx, types.DefaultParams())

	return app, ctx
}

func setup(isCheckTx bool) *jklapp.JackalApp {
	app, genesisState := genApp(!isCheckTx, 5)
	if !isCheckTx {
		// WO-69 test scaffolding: seed one bonded dummy validator (test genesis only)
		bondAmt := sdk.TokensFromConsensusPower(100, sdk.DefaultPowerReduction)
		privKey := ed25519.GenPrivKeyFromSecret([]byte("wo69-dummy-validator"))
		pubKeyAny, err := codectypes.NewAnyWithValue(privKey.PubKey())
		if err != nil {
			panic(err)
		}
		appCodec := app.AppCodec()

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
		// init chain must be called to stop deliverState from being nil
		stateBytes, err := json.MarshalIndent(genesisState, "", " ")
		if err != nil {
			panic(err)
		}

		// Initialize the chain
		app.InitChain(
			abci.RequestInitChain{
				Validators:      []abci.ValidatorUpdate{},
				ConsensusParams: simtestutil.DefaultConsensusParams,
				AppStateBytes:   stateBytes,
			},
		)
	}

	return app
}

func genApp(withGenesis bool, invCheckPeriod uint) (*jklapp.JackalApp, jklapp.GenesisState) {
	db := dbm.NewMemDB()
	encCdc := jklapp.MakeEncodingConfig()
	app := jklapp.NewJackalApp(
		log.NewNopLogger(),
		db,
		nil,
		true,
		map[int64]bool{},
		jklapp.DefaultNodeHome,
		invCheckPeriod,
		encCdc,
		simtestutil.EmptyAppOptions{},
		jklapp.GetWasmOpts(simtestutil.EmptyAppOptions{}),
	)

	if withGenesis {
		return app, jklapp.NewDefaultGenesisState()
	}

	return app, jklapp.GenesisState{}
}
