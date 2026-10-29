package app

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// freeMessages lists the message types Jackal nodes never charge fees for.
// Ported verbatim from the JackalLabs SDK fork (cosmos-sdk-new v0.45.17,
// x/auth/ante/fee.go) so fee and gas behaviour is identical across the
// v0.47 upgrade.
const (
	MsgPostProof              = "/canine_chain.storage.MsgPostProof"
	MsgRequestAttestationForm = "/canine_chain.storage.MsgRequestAttestationForm"
	MsgAttest                 = "/canine_chain.storage.MsgAttest"
	MsgReport                 = "/canine_chain.storage.MsgReport"
)

var freeMessages = map[string]bool{
	MsgPostProof:              true,
	MsgRequestAttestationForm: true,
	MsgAttest:                 true,
	MsgReport:                 true,
}

// FreeFeeDecorator is the v0.47 DeductFeeDecorator (x/auth/ante/fee.go) with
// the Jackal free-message whitelist spliced in: txs made only of freeMessages
// are never charged, but must still declare a fee meeting min-gas-prices
// (fork MempoolFeeDecorator behaviour = CheckTx policy option 1). The
// TxFeeChecker parameterisation is dropped: the fee check is inlined on the
// declared fee.
type FreeFeeDecorator struct {
	accountKeeper  ante.AccountKeeper
	bankKeeper     authtypes.BankKeeper
	feegrantKeeper ante.FeegrantKeeper
}

func NewFreeFeeDecorator(ak ante.AccountKeeper, bk authtypes.BankKeeper, fk ante.FeegrantKeeper) FreeFeeDecorator {
	return FreeFeeDecorator{
		accountKeeper:  ak,
		bankKeeper:     bk,
		feegrantKeeper: fk,
	}
}

func (dfd FreeFeeDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return ctx, sdkerrors.Wrap(sdkerrors.ErrTxDecode, "Tx must be a FeeTx")
	}

	if dfd.accountKeeper.GetModuleAddress(authtypes.FeeCollectorName) == nil {
		return ctx, fmt.Errorf("fee collector module account (%s) has not been set", authtypes.FeeCollectorName)
	}

	if !simulate && ctx.BlockHeight() > 0 && feeTx.GetGas() == 0 {
		return ctx, sdkerrors.Wrap(sdkerrors.ErrInvalidGasLimit, "must provide positive gas")
	}

	// Declared fee, captured once: used for the min-fee check, feegrant,
	// deduction and events (fork-exact, round 5 film).
	fee := feeTx.GetFee()

	// Min-gas-price check, inlined. Net condition matches both the fork's
	// MempoolFeeDecorator (IsCheckTx && !simulate) and the v0.47 default
	// (checker invoked only when !simulate, guards IsCheckTx): identical.
	if !simulate && ctx.IsCheckTx() {
		minGasPrices := ctx.MinGasPrices()
		if !minGasPrices.IsZero() {
			requiredFees := make(sdk.Coins, len(minGasPrices))
			glDec := sdkmath.LegacyNewDec(int64(feeTx.GetGas()))
			for i, gp := range minGasPrices {
				req := gp.Amount.Mul(glDec)
				requiredFees[i] = sdk.NewCoin(gp.Denom, req.Ceil().RoundInt())
			}
			if !fee.IsAnyGTE(requiredFees) {
				return ctx, sdkerrors.Wrapf(sdkerrors.ErrInsufficientFee, "insufficient fees; got: %s required: %s", fee, requiredFees)
			}
		}
	}

	msgs := tx.GetMsgs()
	shouldBeFree := true
	for _, msg := range msgs {
		url := sdk.MsgTypeURL(msg)
		if !freeMessages[url] {
			shouldBeFree = false
			break
		}
	}

	feePayer := feeTx.FeePayer()
	feeGranter := feeTx.FeeGranter()
	deductFeesFrom := feePayer

	if feeGranter != nil {
		if dfd.feegrantKeeper == nil {
			return ctx, sdkerrors.Wrap(sdkerrors.ErrInvalidRequest, "fee grants are not enabled")
		} else if !feeGranter.Equals(feePayer) {
			err := dfd.feegrantKeeper.UseGrantedFees(ctx, feeGranter, feePayer, fee, tx.GetMsgs())
			if err != nil {
				return ctx, sdkerrors.Wrapf(err, "%s not allowed to pay fees from %s", feeGranter, feePayer)
			}
		}

		deductFeesFrom = feeGranter
	}

	deductFeesFromAcc := dfd.accountKeeper.GetAccount(ctx, deductFeesFrom)
	if deductFeesFromAcc == nil {
		return ctx, sdkerrors.Wrapf(sdkerrors.ErrUnknownAddress, "fee payer address: %s does not exist", deductFeesFrom)
	}

	// deduct the fees
	if !fee.IsZero() && !shouldBeFree {
		err := ante.DeductFees(dfd.bankKeeper, ctx, deductFeesFromAcc, fee)
		if err != nil {
			return ctx, err
		}
	}

	events := sdk.Events{
		sdk.NewEvent(
			sdk.EventTypeTx,
			sdk.NewAttribute(sdk.AttributeKeyFee, fee.String()),
			sdk.NewAttribute(sdk.AttributeKeyFeePayer, deductFeesFrom.String()),
		),
	}
	ctx.EventManager().EmitEvents(events)

	newCtx := ctx.WithPriority(0) // fork-exact: v0.45 had no tx priority
	return next(newCtx, tx, simulate)
}
