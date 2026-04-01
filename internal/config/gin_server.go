package config

const (
	// DebugMode indicates gin mode is debug.
	DebugMode GinMod = "debug"
	// ReleaseMode indicates gin mode is release.
	ReleaseMode GinMod = "release"
	// TestMode indicates gin mode is test.
	TestMode GinMod = "test"
)

type GinMod string

type GinServerConfig struct {
	Host   string
	Port   string
	AppMod GinMod
}
