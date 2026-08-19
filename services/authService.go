package services

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/routing"
	"github.com/goyourt/yogourt/services/database"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
)

func Authenticate(c *gin.Context, currentUser interfaces.BaseInterface) {
	token, err := GetRequestToken(c)
	if err != nil {
		// The reason (missing header, malformed header…) stays server-side:
		// the authorization chain answers generic bodies only, and the
		// internal error strings of the framework must not describe its
		// structure to an anonymous caller (AUTHZ-014).
		log.Printf("authentication refused: %v", err)
		routing.RespondAndAbort(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	parsedToken, err := ValidToken(token)
	if err != nil {
		log.Printf("authentication refused: %v", err)
		routing.RespondAndAbort(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	userUuid, err := GetUUIDClaim(parsedToken, "uuid")
	if err != nil {
		log.Printf("authentication refused: %v", err)
		routing.RespondAndAbort(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := database.GetOneBy(currentUser, map[string]any{"uuid": userUuid}); err != nil {
		respondUserLookupFailure(c, err)
		return
	}

	setCurrentUser(c, currentUser)
	attachAuthorizationSubject(c, currentUser)
}

// respondUserLookupFailure distinguishes an unknown user (401) from a
// technical database failure (503): an outage must never masquerade as an
// authentication refusal (AUTHZ-014).
func respondUserLookupFailure(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The subject of a validly signed token has no row: an anonymous
		// caller must not learn that, the body stays the generic refusal.
		log.Printf("authentication refused: no user for the token subject")
		routing.RespondAndAbort(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	log.Printf("authentication unavailable: %v", err)
	routing.RespondServiceUnavailable(c)
}

func setCurrentUser(c *gin.Context, currentUser interfaces.BaseInterface) {
	c.Set(providers.ContextCurrentUser, currentUser)
}

// AttachSubject binds the authorization subject to the request context,
// making it visible to the RBAC middleware and to the Context authorization
// helpers. Applications with their own authentication call it after a
// successful login.
func AttachSubject(c *gin.Context, subject authorization.Subject) {
	c.Request = c.Request.WithContext(authorization.WithSubject(c.Request.Context(), subject))
}

// attachAuthorizationSubject derives the authorization subject from the
// authenticated user and attaches it to the request context. A user model
// implementing authorization.SubjectResolver controls its own subject;
// otherwise the subject carries the stable UUID as identity and the internal
// SQL id as attribute. The current-user mechanism is kept unchanged alongside.
func attachAuthorizationSubject(c *gin.Context, currentUser interfaces.BaseInterface) {
	if resolver, ok := currentUser.(authorization.SubjectResolver); ok {
		AttachSubject(c, resolver.AuthorizationSubject())
		return
	}

	uuid := currentUser.GetUuid()
	if uuid == "" {
		return
	}
	AttachSubject(c, authorization.Subject{
		ID:         uuid,
		Attributes: map[string]any{"internal_id": currentUser.GetID()},
	})
}
