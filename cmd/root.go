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

// rootCmd represents the base command when called without any subcommands
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

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.chilly-boy.yaml)")
	rootCmd.PersistentFlags().String("i2cbus", "", "name of the i2c bus")
	rootCmd.PersistentFlags().String("spibus", "", "name of the spi bus")
	rootCmd.PersistentFlags().String("mqtt-broker", "", "mqtt broker url")
	rootCmd.PersistentFlags().Duration("pid-history-offset", 1*time.Minute, "PID historical offset interval")
	rootCmd.PersistentFlags().Duration("pid-interval", 100*time.Millisecond, "PID loop/sensor polling interval")
	//rootCmd.PersistentFlags().Duration("light-interval", 100*time.Millisecond, "Light sensor polling interval")
	//rootCmd.PersistentFlags().Duration("rtd-interval", 100*time.Millisecond, "Temperature sensor polling interval")
	rootCmd.PersistentFlags().Duration("temp-interval", 6*time.Second, "Reference temperature sensor polling interval")
	rootCmd.PersistentFlags().Duration("watchdog-timeout", 10*time.Second, "Chiller shutdown timeout without light readings")
	rootCmd.PersistentFlags().Float64("pid-kp", 0.0, "PID Kp")
	rootCmd.PersistentFlags().Float64("pid-ki", 0.0, "PID Ki")
	rootCmd.PersistentFlags().Float64("pid-kd", 0.0, "PID Kd")
	//rootCmd.PersistentFlags().Float64("awg", 0.0, "PID Anti-Windup Gain")
	rootCmd.PersistentFlags().Float64("startup-light-ratio", 45, "Startup light target as a percentage of peak")
	rootCmd.PersistentFlags().Float64("peak-light", 4400, "Estimated peak light level")
	//rootCmd.PersistentFlags().Float64("ema-priming-value", 50, "Value to prime the exponential moving average")

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
