# Yogourt

## Installation du CLI

Pour installer le CLI de Yogourt globalement sur votre machine:

```sh
go install github.com/goyourt/yogourt/cli@latest
```

Cela installera le binaire dans votre `$GOPATH/bin` (ou `$GOBIN` si défini). Assurez-vous que ce dossier est dans votre `PATH` pour pouvoir utiliser la commande depuis n'importe où.

## Utilisation

Après installation, vous pouvez lancer le CLI avec:

```sh
yogourt
```

ou selon le nom du binaire généré (par défaut, le nom du dossier `cli`).

Pour afficher l'aide:

```sh
yogourt --help
```

## Features

### Yogourt init

When the project is initialized, many files are generated for you:

#### Config File

A config file `config.yaml` is generated at the root of the project with default values.
Use this file to configure your app (database, server port, etc).

#### Docker Compose

A `docker-compose.yml` file is generated at the root of the project with multiple services.
- Database (PostgreSQL)
- Redis (Cache database)
- Volumes

To use your Yogourt app, you need to start these containers with:
```sh
docker-compose up -d
```

otherwise, you need to configure your own database and cache and pass the information in the `config.yaml` file.

#### Auth system

A fully fonctional auth system is generated for you, with:
- Models and database tables
- Sign up and login routes
- Password hashing
- JWT token generation and validation

### Routing system

Yogourt uses a routing system very similar to Next.js framework.

To create a route you need to create a folder in the `api` folder.

Yogourt automatically search for `GET`, `PUT`, `POST`, `PATCH`, `DELETE` functions in any go module of the route folder.

To be accepted by Yogourt, every go file inside of the `api` folder must be a module. This means your files must be in the `package main` and have an empty `main` function.

To create a sub-route, you need to create a folder inside of the route folder.

To implement route with parameters, you need to create a folder and put the parameter name between brackets like this `[id]`.

### Middlewares

In your generated files, you will find a `middlewares` folder with a `middleware.go` file.

To add a middleware to a route, you juste have to add the function in the `Callbacks` var of `middleware.go` file.

The following rules are used to add middlewares to routes:
- middlewares are stacked, this mean that if you set a middleware to the route `/api` and another to the route `/api/user`, both middlewares will be executed for the route `/api/user`
- you can delete middlewares for a specific route, and it's sub-routes by bindin the route middleware to `nil`
- if you put the char `^` at the beginning of your route name (ex `^/api/user/login`), middlewares of the parents routes will not be executed for this route and it's sub-routes