package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

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

type Config struct {
	ProjectPath string
	FilterText  string
	ShowCounts  bool
	Month       *string // YYYY-MM format, nil means all months
}

func parseArgs() (*Config, error) {
	config := &Config{}
	args := os.Args[1:]

	if len(args) == 0 {
		return nil, fmt.Errorf("missing project path")
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "-f" || arg == "--filter" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("filter flag requires a value")
			}
			config.FilterText = args[i+1]
			i += 2
		} else if arg == "-c" || arg == "--counts" {
			config.ShowCounts = true
			i++
		} else if arg == "-m" || arg == "--month" {
			// Check if next arg exists and is not a flag
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				month := args[i+1]
				config.Month = &month
				i += 2
			} else {
				// No value provided, use current month
				currentMonth := time.Now().Format("2006-01")
				config.Month = &currentMonth
				i++
			}
		} else if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("unknown flag: %s", arg)
		} else {
			// This should be the project path
			if config.ProjectPath == "" {
				config.ProjectPath = arg
				i++
			} else {
				return nil, fmt.Errorf("unexpected argument: %s", arg)
			}
		}
	}

	if config.ProjectPath == "" {
		return nil, fmt.Errorf("missing project path")
	}

	// Validate month format if provided
	if config.Month != nil {
		if _, err := time.Parse("2006-01", *config.Month); err != nil {
			return nil, fmt.Errorf("invalid month format '%s', expected YYYY-MM", *config.Month)
		}
	}

	return config, nil
}

func getTerminalWidth() int {
	type winsize struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}

	ws := &winsize{}
	retCode, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))

	if int(retCode) == -1 || errno != 0 {
		// Terminal width not available (pipe, non-interactive, etc.)
		return 80
	}

	return int(ws.Col)
}

func main() {
	config, err := parseArgs()
	if err != nil {
		fmt.Printf("Error: %v\n\n", err)
		fmt.Println("Usage: hugo-calendar <path-to-hugo-project> [options]")
		fmt.Println("Options:")
		fmt.Println("  -f, --filter TEXT    Exclude posts containing TEXT in their body")
		fmt.Println("  -c, --counts         Show post counts instead of day numbers")
		fmt.Println("  -m, --month YYYY-MM  Show only the specified month (default: current month)")
		os.Exit(1)
	}

	postsPath := filepath.Join(config.ProjectPath, "content", "posts")

	// Check if posts directory exists
	if _, err := os.Stat(postsPath); os.IsNotExist(err) {
		fmt.Printf("Posts directory not found: %s\n", postsPath)
		os.Exit(1)
	}

	// Parse all posts and count by date
	postCounts, err := parsePostsAndCount(postsPath, config.FilterText)
	if err != nil {
		fmt.Printf("Error parsing posts: %v\n", err)
		os.Exit(1)
	}

	if len(postCounts) == 0 {
		fmt.Println("No posts found in the Hugo project.")
		return
	}

	// Render calendar
	renderCalendars(postCounts, config.ShowCounts, config.Month)
}

func parsePostsAndCount(postsPath, filterText string) (map[string]int, error) {
	postCounts := make(map[string]int)

	err := filepath.Walk(postsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Look for index.md files
		if info.Name() == "index.md" {
			frontMatter, postBody, err := parsePostFile(path)
			if err != nil {
				fmt.Printf("Warning: Could not parse post file %s: %v\n", path, err)
				return nil // Continue processing other files
			}

			// Skip draft posts
			if frontMatter.Draft {
				return nil
			}

			// Skip posts containing filter text in body
			if filterText != "" && strings.Contains(postBody, filterText) {
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

func parsePostFile(filePath string) (*PostFrontMatter, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var frontMatterLines []string
	var bodyLines []string
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
				continue
			}
		}

		if inFrontMatter && !frontMatterEnded {
			frontMatterLines = append(frontMatterLines, line)
		} else if frontMatterEnded {
			bodyLines = append(bodyLines, line)
		}
	}

	if !frontMatterEnded {
		return nil, "", fmt.Errorf("front matter not properly closed")
	}

	frontMatterYAML := strings.Join(frontMatterLines, "\n")
	var frontMatter PostFrontMatter
	err = yaml.Unmarshal([]byte(frontMatterYAML), &frontMatter)
	if err != nil {
		return nil, "", err
	}

	postBody := strings.Join(bodyLines, "\n")
	return &frontMatter, postBody, nil
}

func renderCalendars(postCounts map[string]int, showCounts bool, monthFilter *string) {
	var months []time.Time

	if monthFilter != nil {
		// Single month mode - parse the target month
		targetMonth, err := time.Parse("2006-01", *monthFilter)
		if err != nil {
			fmt.Printf("Error parsing month filter: %v\n", err)
			return
		}
		months = append(months, time.Date(targetMonth.Year(), targetMonth.Month(), 1, 0, 0, 0, 0, time.UTC))
	} else {
		// Original behavior - show all months with posts
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

		// Need to import sort for this
		// Find min and max dates
		minDate := dates[0]
		maxDate := dates[0]
		for _, date := range dates {
			if date.Before(minDate) {
				minDate = date
			}
			if date.After(maxDate) {
				maxDate = date
			}
		}

		// Generate all months in range
		current := time.Date(minDate.Year(), minDate.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(maxDate.Year(), maxDate.Month(), 1, 0, 0, 0, 0, time.UTC)

		for !current.After(end) {
			months = append(months, current)
			current = current.AddDate(0, 1, 0)
		}
	}

	// Render calendars in rows
	renderCalendarGrid(months, postCounts, showCounts)
}

func renderCalendarGrid(months []time.Time, postCounts map[string]int, showCounts bool) {
	// Calculate terminal width and calendars per row
	const calendarWidth = 22 // Each calendar is 20 chars wide + 2 chars padding
	terminalWidth := getTerminalWidth()
	calendarsPerRow := terminalWidth / calendarWidth

	// Ensure at least one calendar per row
	if calendarsPerRow < 1 {
		calendarsPerRow = 1
	}

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
			grid := generateCalendarGrid(month, postCounts, white, brightGreen, showCounts)
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

func generateCalendarGrid(month time.Time, postCounts map[string]int, white, brightGreen *color.Color, showCounts bool) []string {
	var grid []string

	// First day of month and its weekday
	firstDay := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	startWeekday := int(firstDay.Weekday()) // 0 = Sunday

	// Last day of month
	lastDay := firstDay.AddDate(0, 1, -1)
	daysInMonth := lastDay.Day()

	// Get current date for underlining
	today := time.Now()
	currentDateKey := today.Format("2006-01-02")

	// Build calendar grid with proper alignment
	day := 1
	weekRow := 0

	for day <= daysInMonth || weekRow == 0 {
		var rowParts []string

		// For each column (weekday) in this row
		for col := 0; col < 7; col++ {
			if weekRow == 0 && col < startWeekday {
				// Empty cell before month starts
				if showCounts {
					rowParts = append(rowParts, "  ")
				} else {
					rowParts = append(rowParts, "  ")
				}
			} else if day <= daysInMonth {
				// Valid day in month
				dateKey := time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
				count := postCounts[dateKey]
				isToday := dateKey == currentDateKey

				var dayStr string
				if showCounts {
					if count > 0 {
						if isToday {
							dayStr = color.New(color.FgBlack, color.BgWhite).Sprintf("%2d", count)
						} else {
							dayStr = brightGreen.Sprintf("%2d", count)
						}
					} else {
						if isToday {
							dayStr = color.New(color.FgBlack, color.BgWhite).Sprintf(" 0")
						} else {
							dayStr = white.Sprintf(" 0")
						}
					}
				} else {
					if count > 0 {
						if isToday {
							dayStr = color.New(color.FgBlack, color.BgWhite).Sprintf("%2d", day)
						} else {
							dayStr = brightGreen.Sprintf("%2d", day)
						}
					} else {
						if isToday {
							dayStr = color.New(color.FgBlack, color.BgWhite).Sprintf("%2d", day)
						} else {
							dayStr = white.Sprintf("%2d", day)
						}
					}
				}
				rowParts = append(rowParts, dayStr)
				day++
			} else {
				// Empty cell after month ends
				if showCounts {
					rowParts = append(rowParts, "  ")
				} else {
					rowParts = append(rowParts, "  ")
				}
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
