package cmd

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"immich-album-generator/pkg/immich"
	"immich-album-generator/pkg/sync"
)

var (
	url               string
	apiKey            string
	recentDays        int
	recentLimit       int
	recentHalfLife    float64
	recentAlbumName   string
	memoriesDays      int
	memoriesLimit     int
	memoriesHalfLife  float64
	memoriesAlbumName string
	people            []string
	peopleLimit       int
	peopleHalfLife    float64
	peopleAlbumName   string
	dryRun            bool
	verbose           bool
)

var rootCmd = &cobra.Command{
	Use:   "immich-album-generator",
	Short: "Automatically generate Immich Albums based on criteria.",
	Long: `A CLI to fetch images from an Immich server based on 
specific criteria (Recent, Memories, People) and sync them into albums
using an exponential decay weighted random selection algorithm when limits are hit.`,
	RunE: runFunc,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Try to load .env file if it exists
	_ = godotenv.Load()

	rootCmd.Flags().StringVar(&url, "url", os.Getenv("IMMICH_URL"), "Immich server URL (default from IMMICH_URL env)")
	rootCmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("IMMICH_API_KEY"), "Immich API Key (default from IMMICH_API_KEY env)")

	// Recent Flags
	rootCmd.Flags().IntVar(&recentDays, "recent-days", 90, "Recent Images: N days from today")
	rootCmd.Flags().IntVar(&recentLimit, "recent-limit", 5000, "Recent Images: Maximum images")
	rootCmd.Flags().Float64Var(&recentHalfLife, "recent-half-life", 30.0, "Recent Images: Exponential decay half-life in days")
	rootCmd.Flags().StringVar(&recentAlbumName, "recent-album-name", "ScreensaverRecent", "Recent Images: Album name")

	// Memories Flags
	rootCmd.Flags().IntVar(&memoriesDays, "memories-days", 14, "Memories: +/- X days from today's date in past years")
	rootCmd.Flags().IntVar(&memoriesLimit, "memories-limit", 5000, "Memories: Maximum images")
	rootCmd.Flags().Float64Var(&memoriesHalfLife, "memories-half-life", 7.0, "Memories: Exponential decay half-life in days")
	rootCmd.Flags().StringVar(&memoriesAlbumName, "memories-album-name", "ScreensaverMemories", "Memories: Album name")

	// People Flags
	rootCmd.Flags().StringSliceVar(&people, "people", []string{}, "People: Comma separated list of people names or IDs")
	rootCmd.Flags().IntVar(&peopleLimit, "people-limit", 5000, "People: Maximum images")
	rootCmd.Flags().Float64Var(&peopleHalfLife, "people-half-life", 3650.0, "People: Exponential decay half-life in days from today")
	rootCmd.Flags().StringVar(&peopleAlbumName, "people-album-name", "ScreensaverPeople", "People: Album name")

	// Global Flags
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without making any API calls to create or sync albums")
	rootCmd.Flags().BoolVar(&verbose, "verbose", false, "Log every Immich API request and response to stderr")
}

func runFunc(cmd *cobra.Command, args []string) error {
	if url == "" || apiKey == "" {
		return fmt.Errorf("--url (or IMMICH_URL) and --api-key (or IMMICH_API_KEY) are required")
	}

	fmt.Println("Starting Immich Album Generator sync process...")

	client := immich.NewClient(url, apiKey)
	client.Verbose = verbose

	if recentLimit > 0 {
		if err := sync.SyncRecent(client, recentAlbumName, recentDays, recentLimit, recentHalfLife, dryRun); err != nil {
			fmt.Printf("Error syncing Recent Album: %v\n", err)
		}
	}

	if memoriesLimit > 0 {
		if err := sync.SyncMemories(client, memoriesAlbumName, memoriesDays, memoriesLimit, memoriesHalfLife, dryRun); err != nil {
			fmt.Printf("Error syncing Memories Album: %v\n", err)
		}
	}

	if len(people) > 0 {
		if err := sync.SyncPeople(client, people, peopleAlbumName, peopleLimit, peopleHalfLife, dryRun); err != nil {
			fmt.Printf("Error syncing People Album: %v\n", err)
		}
	}

	fmt.Println("Sync process finished.")
	return nil
}
