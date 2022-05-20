/*
Copyright © 2022 yu-yk

*/
package cmd

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	bc "github.com/yu-yk/bc"
)

var (
	// flags
	download bool
	pool     int
	workers  int
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use: "start",
	Run: start,
}

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	rootCmd.AddCommand(startCmd)

	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	defaultDownloadFlag, defaultWorkers, defaultPool := false, 10, 100
	startCmd.Flags().BoolVarP(&download, "download", "d", defaultDownloadFlag, "download the video")
	startCmd.Flags().IntVarP(&workers, "worker", "w", defaultWorkers, "number of workers")
	startCmd.Flags().IntVarP(&pool, "pool", "p", defaultPool, "pool size")
}

func start(cmd *cobra.Command, args []string) {
	// Create directory.
	if err := bc.Mkdir(viper.GetString("downloads_path")); err != nil {
		panic(err)
	}
	if err := bc.Mkdir(viper.GetString("response_path")); err != nil {
		panic(err)
	}

	// Set error log file.
	errorLogFile, err := os.OpenFile(viper.GetString("log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	log.SetOutput(errorLogFile)

	// Set ffmpeg error log file.
	ffmpegErrLogFile, err := os.OpenFile(viper.GetString("ffmpeg_log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	url := viper.GetString("url")
	accountID := viper.GetString("account_id")
	c := bc.Crawler{
		URL:              strings.ReplaceAll(url, ":account_id", accountID),
		Headers:          viper.GetStringMapString("header"),
		PoolSize:         pool,
		Workers:          workers,
		StartVID:         viper.GetInt("video_id"),
		Step:             viper.GetInt("step"),
		Download:         download,
		DownloadsPath:    viper.GetString("downloads_path"),
		ResponsePath:     viper.GetString("response_path"),
		ErrorLogFile:     errorLogFile,
		FfmpegErrLogFile: ffmpegErrLogFile,
	}

	c.Run()
}
