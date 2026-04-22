package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/timae/ses/internal/db"
	"github.com/timae/ses/internal/shape"
)

var (
	pasteTurn int
	pasteGrep string

	pastesMinSize int
	pastesShape   string
	pastesSince   string
	pastesUntil   string
	pastesProject string
	pastesSource  string
	pastesLimit   int
)

var pasteCmd = &cobra.Command{
	Use:   "paste <id>",
	Short: "Dump user turns (pastes) from a session to stdout",
	Long: `Print the full content of user turns you pasted into a session.
Useful for recovering a big JSON blob, log, or file body you fed Claude
when the assistant can't recall it.

Examples:
  ses paste a3f2                  # every user turn, separated by headers
  ses paste a3f2 --turn 5         # just the 5th user turn
  ses paste a3f2 --grep "ERROR"   # turns matching a regex
  ses paste a3f2 --turn 5 | pbcopy`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session, err := store.GetSession(args[0])
		if err != nil {
			return err
		}
		prompts, err := store.GetUserPrompts(session.ID)
		if err != nil {
			return fmt.Errorf("loading prompts: %w", err)
		}
		if len(prompts) == 0 {
			fmt.Fprintln(os.Stderr, "no user prompts recorded for this session")
			return nil
		}

		var grepRe *regexp.Regexp
		if pasteGrep != "" {
			grepRe, err = regexp.Compile(pasteGrep)
			if err != nil {
				return fmt.Errorf("invalid --grep regex: %w", err)
			}
		}

		if pasteTurn > 0 {
			if pasteTurn > len(prompts) {
				return fmt.Errorf("--turn %d out of range (session has %d user turns)", pasteTurn, len(prompts))
			}
			fmt.Print(prompts[pasteTurn-1])
			if !strings.HasSuffix(prompts[pasteTurn-1], "\n") {
				fmt.Println()
			}
			return nil
		}

		printed := 0
		for i, p := range prompts {
			if grepRe != nil && !grepRe.MatchString(p) {
				continue
			}
			if printed > 0 {
				fmt.Println()
			}
			fmt.Fprintf(os.Stderr, "--- turn %d (%d chars) ---\n", i+1, len(p))
			fmt.Print(p)
			if !strings.HasSuffix(p, "\n") {
				fmt.Println()
			}
			printed++
		}
		if printed == 0 && grepRe != nil {
			fmt.Fprintln(os.Stderr, "no turns matched --grep")
		}
		return nil
	},
}

var pastesCmd = &cobra.Command{
	Use:   "pastes",
	Short: "List big user-pasted content across all sessions",
	Long: `Scan across every session's user turns and list the largest ones.
Use when you know you pasted something but don't remember when or in which
session. Output is one row per turn: session · turn · size · preview · when.

Examples:
  ses pastes                          # default: turns >= 2000 chars
  ses pastes --min 500 --limit 100
  ses pastes --shape json             # only turns that look like JSON
  ses pastes --shape log --since 2026-03-01`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := db.PasteFilter{
			MinSize: pastesMinSize,
			Project: pastesProject,
			Source:  pastesSource,
			Limit:   pastesLimit,
		}
		if pastesSince != "" {
			t, _ := parseDate(pastesSince)
			if !t.IsZero() {
				filter.Since = &t
			}
		}
		if pastesUntil != "" {
			t, _ := parseDate(pastesUntil)
			if !t.IsZero() {
				filter.Until = &t
			}
		}

		var wantShape shape.Shape
		if pastesShape != "" {
			wantShape = shape.Shape(strings.ToLower(pastesShape))
			if !isKnownShape(wantShape) {
				return fmt.Errorf("--shape must be one of: json, log, xml, yaml (got %q)", pastesShape)
			}
		}

		rows, err := store.ListPastes(filter)
		if err != nil {
			return err
		}

		// Shape filter applied after the query — shape is a heuristic, not a
		// column. Truncate to the user's requested limit after filtering so
		// --shape + --limit compose.
		var matched []db.PasteRow
		for _, r := range rows {
			if wantShape != "" && shape.Classify(r.Content) != wantShape {
				continue
			}
			matched = append(matched, r)
			if pastesLimit > 0 && len(matched) >= pastesLimit {
				break
			}
		}

		if len(matched) == 0 {
			fmt.Fprintln(os.Stderr, "no pastes matched the filter")
			return nil
		}
		printPastesTable(matched)
		return nil
	},
}

func isKnownShape(s shape.Shape) bool {
	for _, k := range shape.All() {
		if k == s {
			return true
		}
	}
	return false
}

func printPastesTable(rows []db.PasteRow) {
	fmt.Printf("%-10s  %-5s  %-8s  %-5s  %-16s  %s\n", "SESSION", "TURN", "SIZE", "SHAPE", "WHEN", "PREVIEW")
	for _, r := range rows {
		preview := firstLinePreview(r.Content, 80)
		when := relativeTime(r.SessionStarted)
		fmt.Printf("%-10s  %-5d  %-8s  %-5s  %-16s  %s\n",
			r.SessionShortID,
			r.Ordinal+1,
			humanSize(r.Size),
			string(shape.Classify(r.Content)),
			when,
			preview,
		)
	}
}

func firstLinePreview(s string, maxChars int) string {
	s = strings.TrimLeft(s, " \t\r\n")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	// Truncate by runes so we don't split a multi-byte char.
	count := 0
	for i := range s {
		count++
		if count > maxChars-1 {
			return s[:i] + "…"
		}
	}
	return s
}

func humanSize(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%dB", n)
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("2006-01-02")
}

func init() {
	pasteCmd.Flags().IntVar(&pasteTurn, "turn", 0, "print only the Nth user turn (1-indexed)")
	pasteCmd.Flags().StringVar(&pasteGrep, "grep", "", "print only turns matching this regex")
	rootCmd.AddCommand(pasteCmd)

	pastesCmd.Flags().IntVar(&pastesMinSize, "min", 2000, "minimum paste size in characters")
	pastesCmd.Flags().StringVar(&pastesShape, "shape", "", "filter by detected shape: json|log|xml|yaml")
	pastesCmd.Flags().StringVar(&pastesSince, "since", "", "show pastes from sessions after date (YYYY-MM-DD)")
	pastesCmd.Flags().StringVar(&pastesUntil, "until", "", "show pastes from sessions before date (YYYY-MM-DD)")
	pastesCmd.Flags().StringVar(&pastesProject, "project", "", "filter by project path (substring)")
	pastesCmd.Flags().StringVar(&pastesSource, "source", "", "filter by source (claude|codex)")
	pastesCmd.Flags().IntVar(&pastesLimit, "limit", 50, "max rows to show")
	rootCmd.AddCommand(pastesCmd)
}
