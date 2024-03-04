/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/mikesmitty/chilly-boy/pkg/chillyboy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "chilly-boy",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: chillyboy.Root(),
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.chilly-boy.yaml)")
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	rootCmd.PersistentFlags().String("i2cbus", "", "name of the i2c bus")
	rootCmd.PersistentFlags().String("spibus", "", "name of the spi bus")
	rootCmd.PersistentFlags().String("mqtt-broker", "", "mqtt broker url")
	rootCmd.PersistentFlags().String("pid-algorithm", "", "PID algorithm (classic, pessen-integral, some-overshoot, no-overshoot)")
	rootCmd.PersistentFlags().Duration("pid-interval", 100*time.Millisecond, "PID loop/sensor polling interval")
	rootCmd.PersistentFlags().Duration("pid-lp", 1*time.Second, "PID derivative low pass filter time constant")
	rootCmd.PersistentFlags().Duration("watchdog-timeout", 10*time.Second, "Chiller shutdown timeout without light readings")
	rootCmd.PersistentFlags().Float64("pid-ku", 0.0, "PID Ku")
	rootCmd.PersistentFlags().Float64("pid-tu", 0.0, "PID Tu")
	rootCmd.PersistentFlags().Float64("pid-tune-kp", 0.0, "begin PID tuning with this Kp gain value")
	rootCmd.PersistentFlags().Float64("pid-tune-amp", 0.0, "minimum oscillation amplitude for PID tuning")
	rootCmd.PersistentFlags().Float64("pid-tune-base", 20.0, "baseline cooling level for PID tuning")
	rootCmd.PersistentFlags().Float64("pid-kp", 0.0, "PID Kp")
	rootCmd.PersistentFlags().Float64("pid-ki", 0.0, "PID Ki")
	rootCmd.PersistentFlags().Float64("pid-kd", 0.0, "PID Kd")
	rootCmd.PersistentFlags().Float64("pid-ff", 10.0, "PID FeedForward Gain")
	rootCmd.PersistentFlags().Float64("pid-awg", 0.25, "PID Anti-Windup Gain")
	rootCmd.PersistentFlags().Float64("pid-max-output", 60.0, "PID heater output percentage limit")
	rootCmd.PersistentFlags().Float64("pid-min-output", -100.0, "PID cooler output percentage limit")
	rootCmd.PersistentFlags().Float64("startup-light-ratio", 90, "Startup light target as a percentage of max")
	rootCmd.PersistentFlags().Int("mqtt-sample-interval", 5, "mqtt sample-interval for publishing sensor data")

	viper.BindPFlags(rootCmd.PersistentFlags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".chilly-boy" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".chilly-boy")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
