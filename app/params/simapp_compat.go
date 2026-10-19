package params

// MakeTestEncodingConfig creates an EncodingConfig for a non-amino based test
// configuration. Bridge-compatibility alias for MakeEncodingConfig, named after
// the SDK simapp/params symbol it replaces for the v0.47 upgrade.
// Body equivalence verified against SDK v0.47.8 simapp/params/proto.go.
func MakeTestEncodingConfig() EncodingConfig {
	return MakeEncodingConfig()
}
