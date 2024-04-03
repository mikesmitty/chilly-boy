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
	rootCmd.PersistentFlags().String("pid-algorithm", "classic", "PID algorithm (classic, pessen-integral, some-overshoot, no-overshoot)")
	rootCmd.PersistentFlags().Duration("pid-interval", 100*time.Millisecond, "PID loop/sensor polling interval")
	rootCmd.PersistentFlags().Duration("pid-lp", 1*time.Second, "PID derivative low pass filter time constant")
	rootCmd.PersistentFlags().Duration("watchdog-timeout", 10*time.Second, "Chiller shutdown timeout without light readings")
	rootCmd.PersistentFlags().Float64("max-cool", 100.0, "Peltier max chiller output (0-100)")
	rootCmd.PersistentFlags().Float64("max-heat", 100.0, "Peltier max heater output (0-100)")
	rootCmd.PersistentFlags().Float64("max-light", 0.0, "Peak light level reading for normalization")
	rootCmd.PersistentFlags().Float64("pid-ku", 0.0, "PID Ku")
	rootCmd.PersistentFlags().Float64("pid-tu", 0.0, "PID Tu")
	rootCmd.PersistentFlags().Float64("pid-tune-kp", 0.0, "begin PID tuning with this Kp gain value")
	rootCmd.PersistentFlags().Float64("pid-tune-amp", 0.0, "minimum oscillation amplitude for PID tuning")
	rootCmd.PersistentFlags().Float64("pid-tune-base", 0.0, "baseline cooling level for PID tuning")
	rootCmd.PersistentFlags().Float64("pid-kp", 0.0, "PID Kp")
	rootCmd.PersistentFlags().Float64("pid-ki", 0.0, "PID Ki")
	rootCmd.PersistentFlags().Float64("pid-kd", 0.0, "PID Kd")
	rootCmd.PersistentFlags().Float64("pid-ff", 0.0, "PID Feed-Forward Gain")
	rootCmd.PersistentFlags().Float64("pid-awg", 0.0, "PID Anti-Windup Gain")
	rootCmd.PersistentFlags().Float64("pid-startup-integral", -5.0, "PID startup integral value")
	rootCmd.PersistentFlags().Float64("linear-setpoint-gain", 0.0, "Linear setpoint auto-adjust gain counteracts subtle drift over time")
	rootCmd.PersistentFlags().Float64("linear-setpoint-deadband", 0.0, "Linear setpoint auto-adjust deadband, disables linear setpoint adjustment if normalized light output drift is above/below this value")
	rootCmd.PersistentFlags().Float64("setpoint-step-limit", 0.010, "Setpoint auto-adjust step limit, maximum setpoint value")
	rootCmd.PersistentFlags().Float64("signal-exponent", 14.0, "Exponent for signal input amplification")
	rootCmd.PersistentFlags().Float64("signal-cap", 100000.0, "Cap exponential signal amplification output")
	rootCmd.PersistentFlags().Float64("target-light-ratio", 80.0, "Target light level ratio during cooldown process")
	rootCmd.PersistentFlags().Float64("target-max-light-ratio", 80.0, "Light level threshold ratio where a cooldown process is triggered")
	rootCmd.PersistentFlags().Int("mqtt-sample-interval", 10, "mqtt sample-interval for publishing sensor data")

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
