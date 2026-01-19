package heuristics

// Scoring constants for executable heuristics
const (
	// Positive scores
	ScoreExactMatch   = 120 // Filename exactly matches base name variant
	ScorePartialMatch = 60  // Filename contains base name variant
	ScoreBonusPattern = 80  // Matches known main executable patterns
	ScoreDepthBase    = 10  // Base multiplier for depth scoring
	ScoreLargeFile    = 30  // File size > 10MB
	ScoreMediumFile   = 10  // File size 1-10MB
	ScoreBinDirectory = 20  // Executable in /bin/ directory

	// Negative scores (penalties)
	PenaltyHelper        = -200 // Helper/utility executables
	PenaltyInvalidScript = -300 // Wrapper scripts with invalid build paths
	PenaltyLibrary       = -400 // Shared libraries (.so, .dylib, .dll)
	PenaltySmallFile     = -20  // File size < 100KB
	PenaltyTinyFile      = -50  // File size < 1KB (wrapper scripts)
	PenaltyDeepPath      = -50  // Depth > 10 levels
	PenaltyLibPrefix     = -80  // Files with "lib" prefix

	// Thresholds
	MaxShallowDepth      = 10       // Maximum depth before penalty applies
	DepthScoreOffset     = 11       // Offset for depth score calculation (MaxShallowDepth + 1)
	LargeFileSizeBytes   = 10485760 // 10MB in bytes (10 * 1024 * 1024)
	MediumFileSizeBytes  = 1048576  // 1MB in bytes (1 * 1024 * 1024)
	SmallFileSizeBytes   = 102400   // 100KB in bytes (100 * 1024)
	TinyFileSizeBytes    = 1024     // 1KB in bytes
	MaxScriptSizeBytes   = 10240    // 10KB in bytes (10 * 1024)
	MinNameVariantLength = 3        // Minimum length for name variant matching
)

// bonusPatterns are regex patterns for known main executable names
var bonusPatterns = []string{
	"^wine$", "^wine64$", "^run$", "^start$", "^launch$",
	"^main$", "^app$", "^game$", "^application$",
}

// penaltyPatterns are substrings that indicate helper/utility executables
var penaltyPatterns = []string{
	"chrome-sandbox", "crashpad", "minidump",
	"update", "uninstall", "helper", "crash",
	"debugger", "sandbox", "nacl", "xdg",
	"installer", "setup", "config", "daemon",
	"service", "agent", "monitor", "reporter",
	"dump", "winedump", "windump", "objdump",
	"winedbg", "wineboot", "winecfg", "wineconsole",
	"wineserver", "widl", "wmc", "wrc", "winebuild",
	"winegcc", "wineg++", "winecpp", "winemaker",
	"winefile", "winemine", "winepath",
}

// invalidBuildPatterns are absolute paths that indicate invalid wrapper scripts
var invalidBuildPatterns = []string{
	"/home/runner/",
	"/home/builder/",
	"/tmp/build/",
	"/opt/build/",
	"/workspace/",
	"/build/",
}
