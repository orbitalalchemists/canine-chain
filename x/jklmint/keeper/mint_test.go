package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (suite *MintTestSuite) TestBlockMint() {
	suite.SetupTest()
	app, ctx, k := suite.app, suite.ctx, suite.app.MintKeeper
	denom := k.GetParams(ctx).MintDenom
	feeAccount := app.AccountKeeper.GetModuleAccount(ctx, authtypes.FeeCollectorName)
	feeBalanceBefore, err := app.BankKeeper.Balance(sdk.WrapSDKContext(ctx), &types.QueryBalanceRequest{
		Address: feeAccount.GetAddress().String(),
		Denom:   denom,
	})
	suite.Require().NoError(err)
	suite.Require().Equal(sdk.ZeroInt(), feeBalanceBefore.Balance.Amount)
	supplyBefore, err := app.BankKeeper.TotalSupply(sdk.WrapSDKContext(ctx), &types.QueryTotalSupplyRequest{})
	suite.Require().NoError(err)
	// WO-70 (v0.47): the WO-69 bonded-validator seed puts 100000000 "stake" into test
	// genesis, so TOTAL supply can never start empty - the v0.45 premise is dead.
	// Intent preserved: the MINT DENOM starts absent.
	// ORIGINAL (structurally unsatisfiable on v0.47): suite.Require().True(supplyBefore.Supply.Empty())
	suite.Require().True(supplyBefore.Supply.AmountOf(denom).IsZero(),
		"WO-70: mint-denom supply must start empty; total supply carries the WO-69 seed")
	// We have now proved we started with nothing

	k.BlockMint(ctx)

	feeBalanceAfter, err := app.BankKeeper.Balance(sdk.WrapSDKContext(ctx), &types.QueryBalanceRequest{
		Address: feeAccount.GetAddress().String(),
		Denom:   denom,
	})

	suite.Require().NoError(err)
	suite.Require().Equal(sdk.NewInt(3360000), feeBalanceAfter.Balance.Amount)
	supplyAfter, err := app.BankKeeper.TotalSupply(sdk.WrapSDKContext(ctx), &types.QueryTotalSupplyRequest{})
	suite.Require().NoError(err)
	// ORIGINAL (v0.45 era): suite.Require().Equal(1, len(supplyAfter.Supply))
	// WO-70 (v0.47): post-mint supply necessarily holds the WO-69 seed coin ("stake")
	// beside the minted "ujkl" - exactly 2 entries.
	suite.Require().Equal(2, len(supplyAfter.Supply),
		"WO-70: post-mint supply = seed(stake) + minted(ujkl)")
	suite.Require().Equal(sdk.NewInt(4_200_000), supplyAfter.Supply.AmountOf(denom))
	// After BlockMint we now have exactly 3.6JKL in the fee collector account
}

func (suite *MintTestSuite) TestNoProviderBlockMint() {
	suite.SetupTest()
	app, ctx, k := suite.app, suite.ctx, suite.app.MintKeeper

	params := k.GetParams(ctx)
	params.StorageProviderRatio = 0
	k.SetParams(ctx, params)

	denom := k.GetParams(ctx).MintDenom

	pr := k.GetParams(ctx).StorageProviderRatio
	suite.Require().Equal(int64(0), pr)

	feeAccount := app.AccountKeeper.GetModuleAccount(ctx, authtypes.FeeCollectorName)
	feeBalanceBefore, err := app.BankKeeper.Balance(sdk.WrapSDKContext(ctx), &types.QueryBalanceRequest{
		Address: feeAccount.GetAddress().String(),
		Denom:   denom,
	})
	suite.Require().NoError(err)
	suite.Require().Equal(sdk.ZeroInt(), feeBalanceBefore.Balance.Amount)
	supplyBefore, err := app.BankKeeper.TotalSupply(sdk.WrapSDKContext(ctx), &types.QueryTotalSupplyRequest{})
	suite.Require().NoError(err)
	// WO-70 (v0.47): the WO-69 bonded-validator seed puts 100000000 "stake" into test
	// genesis, so TOTAL supply can never start empty - the v0.45 premise is dead.
	// Intent preserved: the MINT DENOM starts absent.
	// ORIGINAL (structurally unsatisfiable on v0.47): suite.Require().True(supplyBefore.Supply.Empty())
	suite.Require().True(supplyBefore.Supply.AmountOf(denom).IsZero(),
		"WO-70: mint-denom supply must start empty; total supply carries the WO-69 seed")
	// We have now proved we started with nothing

	k.BlockMint(ctx)

	feeBalanceAfter, err := app.BankKeeper.Balance(sdk.WrapSDKContext(ctx), &types.QueryBalanceRequest{
		Address: feeAccount.GetAddress().String(),
		Denom:   denom,
	})

	suite.T().Log(params.TokensPerBlock)
	suite.Require().NoError(err)
	suite.Require().Equal(sdk.NewInt(3360000), feeBalanceAfter.Balance.Amount)
	supplyAfter, err := app.BankKeeper.TotalSupply(sdk.WrapSDKContext(ctx), &types.QueryTotalSupplyRequest{})
	suite.Require().NoError(err)
	// ORIGINAL (v0.45 era): suite.Require().Equal(1, len(supplyAfter.Supply))
	// WO-70 (v0.47): post-mint supply necessarily holds the WO-69 seed coin ("stake")
	// beside the minted "ujkl" - exactly 2 entries.
	suite.Require().Equal(2, len(supplyAfter.Supply),
		"WO-70: post-mint supply = seed(stake) + minted(ujkl)")
	suite.Require().Equal(sdk.NewInt(4_200_000), supplyAfter.Supply.AmountOf(denom))
	// After BlockMint we now have exactly 3.6JKL in the fee collector account
}

func (suite *MintTestSuite) TestDecRatios() {
	suite.SetupTest()

	stakerRatio := sdk.NewDec(80).QuoInt64(100)

	s, err := sdk.NewDecFromStr("0.8")
	suite.Require().NoError(err)

	suite.Require().Equal(s, stakerRatio)

	i := stakerRatio.MulInt64(4_200_000)

	suite.Require().Equal(int64(3360000), i.TruncateInt64())
}
