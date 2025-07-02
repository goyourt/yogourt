# 🥛 Yogourt CLI

# liste des différantes commandes du package yogourt:

- yogourt help : Affiche les différentes commandes
- yogourt init [projectName] [--config cheminDuFichierConfig] : initalisation du projet
- yogourt model : Création d'un model à l'aide d'un wizard
- yogourt migrate [nomDuModele] : Migration du modele vers la base de données

# 📋 Exemple d'utilisation du CLI

# Initialiser un projet avec un fichier de configuration
yogourt init myApiProject --config config/config.yaml

# Créer un modèle interactif
yogourt model 
(suivre les indications du wizard)

# Migrer le modèle "Article" vers la base de données
yogourt migrate Article