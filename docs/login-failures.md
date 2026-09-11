# Échecs de connexion

Les helpers de mot de passe conservent un compteur Redis glissant de 24 heures
pour une identité. Chaque échec est un événement distinct, y compris lorsque
plusieurs échecs arrivent pendant la même seconde ou en parallèle.

`RecordPasswordFailure(ctx, username)` enregistre l'événement, supprime les
événements hors fenêtre, pose une expiration de 24 heures et renvoie le total
dénombré dans cette même opération Redis atomique. Les appels existants restent
disponibles : `SavePasswordFailure(username)` enregistre sans renvoyer le total,
et `GetPasswordFailureCount(username)` lit le total. Les versions avec contexte
doivent être préférées dans un chemin de requête :

```go
count, err := services.RecordPasswordFailure(request.Context(), username)
if err != nil {
    // Traiter l'indisponibilité du cache selon la politique de l'application.
}
```

La fenêtre utilise l’horloge Redis, commune à tous les processus applicatifs.
Le nouveau namespace repart sans les compteurs historiques. Les anciennes clés
brutes ne sont pas supprimées automatiquement : leur nettoyage doit cibler
uniquement les clés identifiées comme appartenant à l’ancien suivi.

Les clés utilisent le namespace `yogourt:password-failures:sha256:` suivi du
SHA-256 de l'identité. Cela évite de placer l'identité brute dans une clé Redis
et sépare ces données des autres usages du cache. Un hash déterministe d'une
identité prévisible ne rend pas cette identité anonyme ; les protections Redis,
la minimisation des données et la durée de rétention restent nécessaires.

Le compteur est une mesure de sécurité et d'observabilité. Il ne constitue pas
un mécanisme d'admission atomique applicatif : une application qui refuse,
temporise ou verrouille un compte doit appliquer sa propre décision atomique,
avec sa politique de seuil, de récupération et les dimensions pertinentes
(compte, origine, appareil, etc.).

Les tests Redis complets sont opt-in pour ne pas toucher à une instance locale
par défaut :

```bash
YOGOURT_TEST_REDIS_ADDR=127.0.0.1:6379 go test ./services -run TestPasswordFailureCounterWithRedis
```

`YOGOURT_TEST_REDIS_PASSWORD` peut être défini si cette instance l'exige. Le
test crée et supprime uniquement ses propres clés dans ce namespace.
