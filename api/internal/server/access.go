package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// Collection role constants (mirror upstream: role_id values stored in
// collection_grant).
const (
	RoleRestricted int16 = 1 // read own granted assets
	RoleFull       int16 = 2 // read every asset in the collection
	RoleManage     int16 = 3 // CRUD on collection contents (assets, labels, stigs)
	RoleOwner      int16 = 4 // grants management on top of Manage
)

// parseInt64Path normalises a CollectionIdPath / AssetIdPath / GrantIdPath
// (which the generator types as plain strings). Returns 0 + false when
// the value is not a positive int64.
func parseInt64Path(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// authorizeCollection enforces both scope and grant role for a
// collection-scoped operation. Returns the requesting user and their
// effective role on the collection. Writes the appropriate error and
// returns ok=false when the request is unauthorised.
//
// The scope parameter is the OpenAPI scope (e.g.
// "stig-manager:collection:read" or "stig-manager:collection"). The
// minRole parameter is the minimum collection role required (1..4).
// Callers with the bare write scope but no grant get 403; callers
// with a grant but the wrong scope get 403 with the missing-scope
// message.
func (s APIServer) authorizeCollection(
	w http.ResponseWriter, r *http.Request,
	collectionID int64, scope string, minRole int16,
) (*auth.User, int64, int16, bool) {
	user, ok := auth.FromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return nil, 0, 0, false
	}
	if !user.HasScope(scope) {
		writeAuthError(w, http.StatusForbidden, "missing required scope: "+scope)
		return nil, 0, 0, false
	}
	if s.Collections == nil || s.Users == nil || s.Grants == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return nil, 0, 0, false
	}
	if _, err := s.Collections.Get(r.Context(), collectionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "collection not found")
			return nil, 0, 0, false
		}
		s.logErr(r, "lookup collection for access", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to verify access")
		return nil, 0, 0, false
	}

	// Upsert the principal so we always have a row to look up grants
	// for. This mirrors the bootstrap upsert in CreateCollection.
	row, err := s.Users.Upsert(r.Context(),
		user.Subject, user.Username, user.Name, user.Email, user.Raw,
	)
	if err != nil {
		s.logErr(r, "upsert principal", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to record principal")
		return nil, 0, 0, false
	}
	role, err := s.Grants.ResolveUserRole(r.Context(), collectionID, row.UserID)
	if err != nil {
		s.logErr(r, "resolve user role", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to resolve role")
		return nil, 0, 0, false
	}
	if role < minRole {
		writeAuthError(w, http.StatusForbidden, "insufficient collection role")
		return nil, 0, 0, false
	}
	return user, row.UserID, role, true
}
