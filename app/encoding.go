package app

import (
	"github.com/cosmos/cosmos-sdk/std"

	ibcsolomachine "github.com/cosmos/ibc-go/v7/modules/light-clients/06-solomachine"
	ibctm "github.com/cosmos/ibc-go/v7/modules/light-clients/07-tendermint"
	ibclocalhost "github.com/cosmos/ibc-go/v7/modules/light-clients/09-localhost"
	"github.com/jackalLabs/canine-chain/v5/app/params"
)

// MakeEncodingConfig creates a new EncodingConfig with all modules registered
func MakeEncodingConfig() params.EncodingConfig {
	encodingConfig := params.MakeEncodingConfig()
	std.RegisterLegacyAminoCodec(encodingConfig.Amino)
	std.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	ModuleBasics.RegisterLegacyAminoCodec(encodingConfig.Amino)
	ModuleBasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	// WO-95: ibc-go v7 moved light clients out of core 02-client; host must register them
	ibcsolomachine.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	ibctm.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	ibclocalhost.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	return encodingConfig
}
