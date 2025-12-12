package heuristics

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/rs/zerolog"
)

// DefaultScorer implements the Scorer interface with standard heuristics
type DefaultScorer struct {
	Logger *zerolog.Logger
}

// NewScorer creates a new DefaultScorer
func NewScorer(logger *zerolog.Logger) *DefaultScorer {
	return &DefaultScorer{
		Logger: logger,
	}
}

// ChooseBest selects the best executable from a list of candidates
func (s *DefaultScorer) ChooseBest(executables []string, baseName, installDir string) string {
	if len(executables) == 0 {
		return ""
	}
	if len(executables) == 1 {
		return executables[0]
	}

	candidates := make([]ExecutableScore, 0, len(executables))

	for _, exe := range executables {
		score := s.ScoreExecutable(exe, baseName, installDir)
		candidates = append(candidates, ExecutableScore{Path: exe, Score: score})

		if s.Logger != nil {
			s.Logger.Debug().
				Str("executable", exe).
				Int("score", score).
				Msg("scored executable candidate")
		}
	}

	// Sort by score descending (highest score first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates[0].Path
}

// ScoreExecutable assigns a score to an executable based on various heuristics
//
//nolint:gocyclo // scoring uses a set of heuristic rules.
func (s *DefaultScorer) ScoreExecutable(execPath, baseName, installDir string) int {
	score := 0
	filename := strings.ToLower(filepath.Base(execPath))
	normalizedBase := strings.ToLower(baseName)
	nameVariants := helpers.GenerateNameVariants(normalizedBase)

	// Calculate relative path and depth
	relPath := strings.TrimPrefix(execPath, installDir)
	relPath = strings.Trim(relPath, "/")
	depth := len(strings.Split(relPath, "/"))

	// Prefer shallow depth (executables in root or first level)
	// Depth 1: +50, Depth 2: +40, Depth 3: +30, etc.
	score += (11 - depth) * 10
	if depth > 10 {
		score -= 50 // Very deep, probably not the main executable
	}

	// Strong match: filename exactly matches any base variant
exactMatchLoop:
	for _, variant := range nameVariants {
		if variant == "" {
			continue
		}
		if filename == variant || filename == variant+".exe" {
			score += 120
			break exactMatchLoop
		}
	}

	// Partial match: filename contains any of the variants
partialMatchLoop:
	for _, variant := range nameVariants {
		if variant == "" || len(variant) < 3 {
			continue
		}
		if strings.Contains(filename, variant) {
			score += 60
			break partialMatchLoop
		}
	}

	// Bonus for known main executable patterns
	bonusPatterns := []string{
		"^wine$", "^wine64$", "^run$", "^start$", "^launch$",
		"^main$", "^app$", "^game$", "^application$",
	}
	for _, pattern := range bonusPatterns {
		matched, matchErr := regexp.MatchString(pattern, filename)
		if matchErr != nil {
			continue
		}
		if matched {
			score += 80
		}
	}

	// Penalize known helper/utility executables
	penaltyPatterns := []string{
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
	for _, pattern := range penaltyPatterns {
		if strings.Contains(filename, pattern) {
			score -= 200 // Heavy penalty for utility executables
		}
	}

	// Strongly penalize shared libraries and lib-prefixed files that slip through
	if strings.HasPrefix(filename, "lib") {
		score -= 80
	}
	if strings.HasSuffix(filename, ".so") || strings.Contains(filename, ".so.") ||
		strings.HasSuffix(filename, ".dylib") || strings.HasSuffix(filename, ".dll") {
		score -= 400
	}

	// Check file size (main executables are usually larger)
	if info, err := os.Stat(execPath); err == nil {
		fileSize := info.Size()

		if fileSize > 10*1024*1024 { // > 10MB
			score += 30 // Likely a main application
		} else if fileSize > 1*1024*1024 { // 1-10MB
			score += 10 // Reasonable size
		} else if fileSize < 100*1024 { // < 100KB
			score -= 20 // Too small, probably a helper

			// Extra penalty for tiny executables (< 1KB) - likely wrapper scripts
			if fileSize < 1024 {
				score -= 50 // Very small, probably a wrapper script
			}
		}
	}

	// Bonus for executables in "bin" directory
	if strings.Contains(strings.ToLower(relPath), "/bin/") {
		score += 20
	}

	// Additional check: penalize if executable is a shell script with invalid references
	if s.isInvalidWrapperScript(execPath) {
		score -= 300 // Heavy penalty for wrapper scripts pointing to invalid paths
	}

	return score
}

// isInvalidWrapperScript checks if file is a wrapper script with invalid path references
func (s *DefaultScorer) isInvalidWrapperScript(execPath string) bool {
	// Only check small files (< 10KB) that might be scripts
	info, err := os.Stat(execPath)
	if err != nil || info.Size() > 10*1024 {
		return false
	}

	// Read first 1KB to check for invalid paths
	file, err := os.Open(execPath)
	if err != nil {
		return false
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	if n == 0 {
		return false
	}

	content := string(buf[:n])

	// Check for shebang (shell script indicator)
	if !strings.HasPrefix(content, "#!") {
		return false // Not a shell script
	}

	// Check for absolute paths that don't exist or point outside installDir
	// Common patterns: /home/runner/, /tmp/build/, /opt/build/, etc.
	invalidPatterns := []string{
		"/home/runner/",
		"/home/builder/",
		"/tmp/build/",
		"/opt/build/",
		"/workspace/",
		"/build/",
	}

	for _, pattern := range invalidPatterns {
		if strings.Contains(content, pattern) {
			if s.Logger != nil {
				s.Logger.Debug().
					Str("executable", execPath).
					Str("invalid_pattern", pattern).
					Msg("detected wrapper script with invalid build path")
			}
			return true
		}
	}

	return false
}
