package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

type PostFrontMatter struct {
	Title string    `yaml:"title"`
	Date  time.Time `yaml:"date"`
	Draft bool      `yaml:"draft"`
}

type PostCount struct {
	Date  time.Time
	Count int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: hugo-calendar <path-to-hugo-project>")
		os.Exit(1)
	}

	projectPath := os.Args[1]
	postsPath := filepath.Join(projectPath, "content", "posts")

	// Check if posts directory exists
	if _, err := os.Stat(postsPath); os.IsNotExist(err) {
		fmt.Printf("Posts directory not found: %s\n", postsPath)
		os.Exit(1)
	}

	// Parse all posts and count by date
	postCounts, err := parsePostsAndCount(postsPath)
	if err != nil {
		fmt.Printf("Error parsing posts: %v\n", err)
		os.Exit(1)
	}

	if len(postCounts) == 0 {
		fmt.Println("No posts found in the Hugo project.")
		return
	}

	// Render calendar
	renderCalendars(postCounts)
}

func parsePostsAndCount(postsPath string) (map[string]int, error) {
	postCounts := make(map[string]int)

	err := filepath.Walk(postsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Look for index.md files
		if info.Name() == "index.md" {
			frontMatter, err := parseFrontMatter(path)
			if err != nil {
				fmt.Printf("Warning: Could not parse front matter in %s: %v\n", path, err)
				return nil // Continue processing other files
			}

			// Skip draft posts
			if frontMatter.Draft {
				return nil
			}

			// Count posts by date (day precision)
			dateKey := frontMatter.Date.Format("2006-01-02")
			postCounts[dateKey]++
		}

		return nil
	})

	return postCounts, err
}

func parseFrontMatter(filePath string) (*PostFrontMatter, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var frontMatterLines []string
	var inFrontMatter bool
	var frontMatterEnded bool

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontMatter {
				inFrontMatter = true
				continue
			} else {
				frontMatterEnded = true
				break
			}
		}

		if inFrontMatter {
			frontMatterLines = append(frontMatterLines, line)
		}
	}

	if !frontMatterEnded {
		return nil, fmt.Errorf("front matter not properly closed")
	}

	frontMatterYAML := strings.Join(frontMatterLines, "\n")
	var frontMatter PostFrontMatter
	err = yaml.Unmarshal([]byte(frontMatterYAML), &frontMatter)
	if err != nil {
		return nil, err
	}

	return &frontMatter, nil
}

func renderCalendars(postCounts map[string]int) {
	// Find date range
	var dates []time.Time
	for dateStr := range postCounts {
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		dates = append(dates, date)
	}

	if len(dates) == 0 {
		return
	}

	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})

	minDate := dates[0]
	maxDate := dates[len(dates)-1]

	// Generate all months in range
	var months []time.Time
	current := time.Date(minDate.Year(), minDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(maxDate.Year(), maxDate.Month(), 1, 0, 0, 0, 0, time.UTC)

	for !current.After(end) {
		months = append(months, current)
		current = current.AddDate(0, 1, 0)
	}

	// Render calendars in rows
	renderCalendarGrid(months, postCounts)
}

func renderCalendarGrid(months []time.Time, postCounts map[string]int) {
	// Calculate terminal width (approximation - each calendar is 20 chars wide + 2 chars padding)
	const calendarWidth = 22
	const terminalWidth = 120 // Assume reasonable terminal width
	calendarsPerRow := terminalWidth / calendarWidth

	white := color.New(color.FgWhite)
	brightGreen := color.New(color.FgHiGreen, color.Bold)

	for i := 0; i < len(months); i += calendarsPerRow {
		end := i + calendarsPerRow
		if end > len(months) {
			end = len(months)
		}

		rowMonths := months[i:end]

		// Print month headers
		for j, month := range rowMonths {
			if j > 0 {
				fmt.Print("  ") // 2-space padding between calendars
			}
			header := month.Format("January 2006")
			white.Printf("%-20s", header)
		}
		fmt.Println()

		// Print day headers
		for j := range rowMonths {
			if j > 0 {
				fmt.Print("  ") // 2-space padding between calendars
			}
			white.Print("Su Mo Tu We Th Fr Sa")
		}
		fmt.Println()

		// Generate calendar grids for this row
		calendarGrids := make([][]string, len(rowMonths))
		maxRows := 0

		for idx, month := range rowMonths {
			grid := generateCalendarGrid(month, postCounts, white, brightGreen)
			calendarGrids[idx] = grid
			if len(grid) > maxRows {
				maxRows = len(grid)
			}
		}

		// Print calendar rows
		for row := 0; row < maxRows; row++ {
			for idx, grid := range calendarGrids {
				if idx > 0 {
					fmt.Print("  ") // 2-space padding between calendars
				}
				if row < len(grid) {
					fmt.Print(grid[row])
				} else {
					fmt.Print(strings.Repeat(" ", 20))
				}
			}
			fmt.Println()
		}

		fmt.Println() // Extra space between calendar rows
	}
}

func generateCalendarGrid(month time.Time, postCounts map[string]int, white, brightGreen *color.Color) []string {
	var grid []string

	// First day of month and its weekday
	firstDay := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	startWeekday := int(firstDay.Weekday()) // 0 = Sunday

	// Last day of month
	lastDay := firstDay.AddDate(0, 1, -1)
	daysInMonth := lastDay.Day()

	// Build calendar grid with proper alignment
	day := 1
	weekRow := 0

	for day <= daysInMonth || weekRow == 0 {
		var rowParts []string

		// For each column (weekday) in this row
		for col := 0; col < 7; col++ {
			if weekRow == 0 && col < startWeekday {
				// Empty cell before month starts
				rowParts = append(rowParts, "  ")
			} else if day <= daysInMonth {
				// Valid day in month
				dateKey := time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
				var dayStr string
				if postCounts[dateKey] > 0 {
					dayStr = brightGreen.Sprintf("%2d", day)
				} else {
					dayStr = white.Sprintf("%2d", day)
				}
				rowParts = append(rowParts, dayStr)
				day++
			} else {
				// Empty cell after month ends
				rowParts = append(rowParts, "  ")
			}
		}

		// Join with single space between columns
		rowString := strings.Join(rowParts, " ")
		grid = append(grid, rowString)
		weekRow++

		// Break if we've processed all days and this row is complete
		if day > daysInMonth {
			break
		}
	}

	return grid
}
