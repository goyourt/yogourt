package main

import (
	"CLI_GO/cmd"
	"fmt"

	"github.com/spf13/cobra"
)

/* --- Fonction Main --- */
func main() {

	/* Commande root */
	var rootCmd = &cobra.Command{
		Use:   "goyourt",
		Short: "GOyourt CLI",
		Long:  "Ceci est un CLI pour le package GOyourt.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Bienvenue dans GOyourt !")
		},
	}

	rootCmd.AddCommand(cmd.InitCmd)  //Ajout de la commande init dans la commande root
	rootCmd.AddCommand(cmd.RouteCmd) //Ajout de la commande route dans la commande root
	rootCmd.AddCommand(cmd.ServeCmd) //Ajout de la commande serve dans la commande root
	rootCmd.AddCommand(cmd.ModelCmd) //Ajout de la commande model dans la commande root

	if rootCmdError := rootCmd.Execute(); rootCmdError != nil {
		fmt.Printf("Erreur de chargement du CLI: %v", rootCmdError)
	}
}
