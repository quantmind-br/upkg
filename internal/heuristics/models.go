package heuristics

// ExecutableScore represents a candidate executable with its calculated score
type ExecutableScore struct {
	Path  string
	Score int
}

// Scorer defines the interface for scoring executables
type Scorer interface {
	// ScoreExecutable calculates a score for a single executable
	ScoreExecutable(path, baseName, installDir string) int

	// ChooseBest selects the best executable from a list of candidates
	ChooseBest(candidates []string, baseName, installDir string) string
}
