# CLI Command Development Guide

## Overview

Cobra-based CLI with factory pattern. Each command is self-contained, receives dependencies via constructor, and initializes services locally within `RunE`.

## Command Template

```go
func NewExampleCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
    var (
        force   bool
        timeout time.Duration
    )

    cmd := &cobra.Command{
        Use:   "example <package>",
        Short: "One-line description",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
            defer cancel()

            // Initialize services locally
            database, err := db.New(cfg, log)
            if err != nil {
                return fmt.Errorf("open database: %w", err)
            }
            defer database.Close()

            registry := backends.NewRegistry(cfg, log)
            tx := transaction.NewManager(log)

            // Command logic here
            return nil
        },
    }

    cmd.Flags().BoolVarP(&force, "force", "f", false, "Force operation")
    cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Operation timeout")

    return cmd
}
```

## Registration

In `root.go` → `NewRootCmd()`:

```go
cmd.AddCommand(NewExampleCmd(cfg, log))
```

## Service Initialization (Inside RunE)

| Service | Initialization |
|---------|----------------|
| Database | `db.New(cfg, log)` + `defer Close()` |
| Registry | `backends.NewRegistry(cfg, log)` |
| Transaction | `transaction.NewManager(log)` |
| Context | `context.WithTimeout(cmd.Context(), ...)` |

## UI Conventions

```go
// Colors (via internal/ui)
ui.Success("Package installed")
ui.Error("Installation failed")
ui.Info("Processing...")

// Tables (via tablewriter)
table := tablewriter.NewWriter(os.Stdout)
table.SetHeader([]string{"Name", "Version"})
table.Render()

// Prompts (via internal/ui)
confirmed, err := ui.Confirm("Proceed?")
```

## Argument Validation

Use Cobra built-ins:
- `cobra.ExactArgs(n)` - exactly n args
- `cobra.MinimumNArgs(n)` - at least n args
- `cobra.MaximumNArgs(n)` - at most n args
- `cobra.RangeArgs(min, max)` - between min and max

## Where to Look

| Command | Reference For |
|---------|---------------|
| `version.go` | Minimal command structure |
| `info.go` | Database queries |
| `install.go` | Full transaction flow |
| `doctor.go` | Interactive prompts |
| `list.go` | Table output |

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Global services | Initialize in RunE |
| `fmt.Printf` | Use `ui.*` helpers |
| Manual arg validation | Use `cobra.*Args` |
| Ignore errors from RunE | Return all errors |
| Skip `defer Close()` | Always close DB/files |
