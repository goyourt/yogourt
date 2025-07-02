/* --- Wizard permettant la création d'un model pour la base de données --- */

package cmd

import (
	"fmt"
	"github.com/goyourt/yogourt/cli/config"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

/* Commande Model */
var ModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Crée un modèle",
	Long:  "Crée un modèle de table lié à la base de données",
	Run: func(cmd *cobra.Command, args []string) {
		CreateModel()
	},
}

// Fonction de création du modele
func CreateModel() {

	// Initialisation des couleurs du text
	green := color.New(color.FgGreen).SprintFunc()
	blue := color.New(color.FgBlue).SprintFunc()

	// Structure d'un champ
	type Field struct {
		Name       string
		Type       string
		Constraint string
	}

	// Structure d'un modele
	type Model struct {
		Name   string
		Fields []Field
	}

	// Initialisation du fichier de logs
	InitLogsFile()

	// Vérification et lecture du fichier config
	cfg, err := config.LoadConfig(ConfigPath)
	if err != nil {
		fmt.Printf(`❌ Fichier config.yaml non trouvé, assurez vous que celui-ci se trouve à la racine de votre projet ou
   que vous avez entré la commande suivante: yogourt init project_name`)
		log.Printf("ERROR: %s\n", err) // Ecriture des logs
		return
	} else {
		// Récupération de la variable d'environnement depuis le fichier config
		ModelFolder := cfg.Paths.ModelFolder

		// Regex pour vérifier que le nom du modele et des champs (lettres en minuscules/majuscules)
		var validName = regexp.MustCompile(`^[a-zA-Z]+$`)

		/* --- Début du wizard --- */
		// Nom du modele
		fmt.Printf("%s ", blue("Quel est le nom du modèle ?\n"))
		var modelName string
		fmt.Scanln(&modelName)
		if modelName == "" || !validName.MatchString(modelName) {
			fmt.Println("❌ Nom invalide, veuillez entrer un nom en lettres uniquement.")
			log.Printf("ERROR: %s\n", err) // Ecriture des logs

			return
		} else {
			fmt.Println(green("Nom du modèle: ", modelName))
		}

		// Type d'ID
		idType := ""
		survey.AskOne(&survey.Select{
			Message: blue("Quel type d'ID voulez-vous ?"),
			Options: []string{"Int (auto-incrémenté)", "UUID"},
		}, &idType)

		// Nombre de champs
		fmt.Printf("%s ", blue("Combien de champs supplémentaires voulez-vous ajouter dans ce modèle ?\n"))
		var fieldCountStr string
		fmt.Scanln(&fieldCountStr)
		fieldCount, err := strconv.Atoi(fieldCountStr) // Convertion de la chaine de caracteres en nombre
		if err != nil || fieldCount <= 0 {
			fmt.Println("❌ Nombre invalide, veuillez entrer un entier positif.")
			log.Printf("ERROR: %s\n", err) // Ecriture des logs
			return
		} else {
			fmt.Println(green("Le modèle " + modelName + " aura " + fieldCountStr + " champs supplémentaires."))
		}

		// Création d'un slice pour les champs
		var fields []Field

		// Nom, type et contraintes des champs
		for i := 1; i <= fieldCount; i++ {
			// Nom du champ
			fmt.Printf("%s ", blue("Quel est le nom de votre champ N°"+strconv.Itoa(i)+" ?\n"))
			var fieldName string
			fmt.Scanln(&fieldName)
			if fieldName == "" || !validName.MatchString(fieldName) {
				fmt.Println("❌ Nom du champ invalide, veuillez entrer un nom en lettres uniquement.")
				return
			}

			// Type du champ (Utilisation de survey pour proposer des choix à l'utilisateur)
			fieldType := ""
			TypePrompt := &survey.Select{
				Message: blue("Choisissez le type du champ :"),
				Options: []string{"string", "int", "bool", "float", "datetime"},
			}
			survey.AskOne(TypePrompt, &fieldType) // Proposition du champ

			// Contraintes du champ (Utilisation de survey pour proposer des choix à l'utilisateur)
			fieldConstraint := ""
			ConstraintPrompt := &survey.Select{
				Message: blue("Ajouter (ou non) une contrainte au champ :"),
				Options: []string{"NOT NULL", "UNIQUE", "UNIQUE & NOT NULL", "Aucune"},
			}
			survey.AskOne(ConstraintPrompt, &fieldConstraint) // Proposition du champ

			// Ajout du champ dans le slice
			field := Field{
				Name:       fieldName,
				Type:       fieldType,
				Constraint: fieldConstraint,
			}

			fields = append(fields, field)

			fmt.Println(green("Champ " + fieldName + " de type " + fieldType + " à été créé avec succès."))
		}

		// Génération du modele
		newModelFile := ModelFolder + modelName + "Model.go"

		modelFile, modelFileError := os.Create(newModelFile)
		if modelFileError != nil {
			fmt.Printf("❌ Erreur lors de la création du model: %s\n", modelFileError)
			log.Printf("ERROR: %s\n", modelFileError) // Ecriture des logs
			return
		}
		defer modelFile.Close()

		// Contenu du modele
		modelFileContent := `package models

import (
	"fmt"
)

`
		modelFileContent += fmt.Sprintf("type %s struct {\n", modelName)
		// Ajout de l'ID selon le type (ID ou UUID)
		if idType == "UUID" {
			modelFileContent += "\tID string \t`gorm:\"type:uuid;default:gen_random_uuid();primaryKey\" json:\"id\"`\n"
		} else {
			modelFileContent += "\tID int \t`gorm:\"primaryKey;autoIncrement;not null;unique\" json:\"id\"`\n"
		}

		// Ajoute les champs
		for _, field := range fields {
			modelFileContent += fmt.Sprintf("\t%s %s ", field.Name, field.Type)
			if field.Constraint != "Aucune" {
				switch field.Constraint {
				case "UNIQUE & NOT NULL":
					modelFileContent += fmt.Sprintf("\t`gorm:\"not null;unique\" json:\"%s\"`\n", field.Name)
				case "NOT NULL":
					modelFileContent += fmt.Sprintf("\t`gorm:\"not null\" json:\"%s\"`\n", field.Name)
				case "UNIQUE":
					modelFileContent += fmt.Sprintf("\t`gorm:\"unique\" json:\"%s\"`\n", field.Name)
				}
			} else {
				modelFileContent += fmt.Sprintf("\t`json:\"%s\"`\n", field.Name)
			}
		}

		modelFileContent += "}\n\n"

		// Ajout des getters et setters pour chaque champs
		for _, field := range fields {
			modelFileContent += fmt.Sprintf("// Champ %s\n", field.Name)
			// Getter
			modelFileContent += fmt.Sprintf("func (u *%s) Get%s() %s {\n", modelName, field.Name, field.Type)
			modelFileContent += fmt.Sprintf("\treturn u.%s\n}\n\n", field.Name)
			// Setter
			modelFileContent += fmt.Sprintf("func (u *%s) Set%s(%s %s) error {", modelName, field.Name, field.Name, field.Type)
			// Contenu du setter selon le type de champ
			switch field.Type {
			case "int":
				modelFileContent += `
	if ` + field.Name + ` < 0 {
		return fmt.Errorf("La valeur ne peut pas être négative")
	}
	u.` + field.Name + ` = ` + field.Name + `
	return nil
}

`
			case "string":
				modelFileContent += `
	if len(` + field.Name + `) == 0 {
		return fmt.Errorf("Le nom ne peut pas être vide")
	}
	u.` + field.Name + ` = ` + field.Name + `
	return nil
}

`
			case "float":
				modelFileContent += `
	if ` + field.Name + ` < 0.00 {
		return fmt.Errorf("La valeur ne peut pas être négative")
	}
	u.` + field.Name + ` = ` + field.Name + `
	return nil
}

`
			case "bool":
				modelFileContent += `
	u.` + field.Name + ` = ` + field.Name + `
	return nil
}

`
			}

		}

		// Ecriture dans le fichier modele
		_, err = modelFile.WriteString(modelFileContent)
		if err != nil {
			fmt.Printf("❌ Erreur d'écriture dans le fichier modèle : %v\n", err)
			log.Printf("ERROR: %s\n", err) // Ecriture des logs
			return
		}

		fmt.Println(green("✅ Modèle " + modelName + " créé avec succès."))

		/* --- Ajout du model dans registry.go --- */
		registryFile := ModelFolder + "registry.go"

		registryFileContent, err := os.ReadFile(registryFile) //Lecture du fichier
		if err != nil {
			fmt.Printf("Erreur lors de la lecture du fichier registry.go: %v \n", err)
			log.Printf("ERROR: %s\n", err) // Ecriture des logs
		}

		// Convertion du contenu en chaîne de caractères
		contentStr := string(registryFileContent)

		// Vérifie si le modèle est déjà présent
		if strings.Contains(contentStr, `"`+modelName+`": &`+modelName+`{}`) {
			fmt.Println("✅ Le modèle est déjà présent dans registry.go")
			return
		}

		// Recherche l'endroit où insérer le nouveau modèle
		mapMarker := "map[string]interface{}{"
		index := strings.Index(contentStr, mapMarker)
		if index == -1 {
			fmt.Println("❌ Impossible de trouver la map Models dans registry.go")
			return
		}

		// Point d'insertion : juste après "map[string]interface{}{"
		insertionPoint := index + len(mapMarker)

		// Construire la nouvelle ligne du modèle
		newEntry := `"` + modelName + `": &` + modelName + `{},`

		// Insertion propre (ajoute avec retour à la ligne et indentation)
		newContent := contentStr[:insertionPoint] + "\n\t" + newEntry + contentStr[insertionPoint:]

		// Réécrire le fichier
		err = os.WriteFile(registryFile, []byte(newContent), 0644)
		if err != nil {
			fmt.Printf("❌ Erreur lors de l'écriture du fichier registry.go: %v\n", err)
			log.Printf("ERROR: %s\n", err) // Ecriture des logs
			return
		}

		fmt.Println("✅ Modèle ajouté à registry.go avec succès")
		log.Printf("Modèle créé avec succès: %s\n", modelName) // Ecriture des logs
	}
}

/* --- Ajout de la commande model à la commande root --- */
func init() {
	rootCmd.AddCommand(ModelCmd)
}
