package main

import (
	"fmt"

	"github.com/goyourt/yogourt/cmd"

	"github.com/spf13/cobra"
)

/* --- Fonction Main --- */
func main() {

	/* Commande root */
	var rootCmd = &cobra.Command{
		Use:   "yogourt",
		Short: "yogourt CLI",
		Long:  "Ceci est un CLI pour le package yogourt.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Bienvenue dans yogourt !")
		},
	}

	rootCmd.AddCommand(cmd.InitCmd)      //Ajout de la commande init dans la commande root
	rootCmd.AddCommand(cmd.RouteCmd)     //Ajout de la commande route dans la commande root
	rootCmd.AddCommand(cmd.ModelCmd)     //Ajout de la commande model dans la commande root
	rootCmd.AddCommand(cmd.MigrationCmd) //Ajout de la commande migration dans la commande root

	if rootCmdError := rootCmd.Execute(); rootCmdError != nil {
		fmt.Printf("Erreur de chargement du CLI: %v", rootCmdError)
	}
}
