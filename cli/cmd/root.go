package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ConfigPath string

var rootCmd = &cobra.Command{
	Use:   "yogourt",
	Short: "yogourt CLI",
	Long:  "Ceci est un CLI pour le package yogourt.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Config utilisée : %s\n", ConfigPath)
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Bienvenue dans yogourt !")
	},
}

func init() {
	// Flag global accessible à toutes les commandes
	rootCmd.PersistentFlags().StringVar(&ConfigPath, "config", "./config.yaml", "Chemin du fichier de configuration")
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}
