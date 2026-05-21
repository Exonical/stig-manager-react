package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// GetGrantsByCollection returns every grant on the collection.
func (s APIServer) GetGrantsByCollection(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, _ api.GetGrantsByCollectionParams) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleFull); !ok {
		return
	}
	if s.Grants == nil {
		writeJSON(w, http.StatusOK, []api.Grant{})
		return
	}
	rows, err := s.Grants.List(r.Context(), collID)
	if err != nil {
		s.logErr(r, "list grants", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list grants")
		return
	}
	out := make([]api.Grant, 0, len(rows))
	for _, g := range rows {
		grant, err := storeToAPIGrant(g)
		if err != nil {
			s.logErr(r, "marshal grant", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to marshal grant")
			return
		}
		out = append(out, grant)
	}
	writeJSON(w, http.StatusOK, out)
}

// PostGrantsByCollection adds one or more grants to the collection.
// Only user-shaped grants are accepted in M6 (group grants land in a
// later milestone).
func (s APIServer) PostGrantsByCollection(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, _ api.PostGrantsByCollectionParams) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleOwner); !ok {
		return
	}
	if s.Grants == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var posts []json.RawMessage
	if err := json.Unmarshal(body, &posts); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	out := make([]api.Grant, 0, len(posts))
	for i, raw := range posts {
		var u api.UserGrant
		if err := json.Unmarshal(raw, &u); err != nil || u.UserId == "" {
			writeAuthError(w, http.StatusBadRequest, "grants["+strconv.Itoa(i)+"]: only user grants supported in this release")
			return
		}
		uid, ok := parseInt64Path(string(u.UserId))
		if !ok {
			writeAuthError(w, http.StatusBadRequest, "grants["+strconv.Itoa(i)+"]: invalid userId")
			return
		}
		role := int16(u.RoleId)
		created, err := s.Grants.Create(r.Context(), store.GrantCreate{
			CollectionID: collID,
			UserID:       uid,
			RoleID:       role,
		})
		if err != nil {
			s.handleGrantWriteErr(w, r, err)
			return
		}
		grant, err := storeToAPIGrant(created)
		if err != nil {
			s.logErr(r, "marshal grant", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to marshal grant")
			return
		}
		out = append(out, grant)
	}
	writeJSON(w, http.StatusCreated, out)
}

// GetGrantByCollectionGrant returns a single grant.
func (s APIServer) GetGrantByCollectionGrant(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, grantId api.GrantIdPath, _ api.GetGrantByCollectionGrantParams) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	gid, ok := parseInt64Path(string(grantId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid grantId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection:read", RoleFull); !ok {
		return
	}
	if s.Grants == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Grants.Get(r.Context(), collID, gid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "grant not found")
			return
		}
		s.logErr(r, "get grant", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get grant")
		return
	}
	grant, err := storeToAPIGrant(row)
	if err != nil {
		s.logErr(r, "marshal grant", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to marshal grant")
		return
	}
	writeJSON(w, http.StatusOK, grant)
}

// PutGrantByCollectionGrant replaces the role of an existing grant.
func (s APIServer) PutGrantByCollectionGrant(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, grantId api.GrantIdPath, _ api.PutGrantByCollectionGrantParams) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	gid, ok := parseInt64Path(string(grantId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid grantId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleOwner); !ok {
		return
	}
	if s.Grants == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.UserGrant
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	row, err := s.Grants.UpdateRole(r.Context(), collID, gid, int16(in.RoleId))
	if err != nil {
		s.handleGrantWriteErr(w, r, err)
		return
	}
	grant, err := storeToAPIGrant(row)
	if err != nil {
		s.logErr(r, "marshal grant", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to marshal grant")
		return
	}
	writeJSON(w, http.StatusOK, grant)
}

// DeleteGrantByCollectionGrant removes a grant. The last Owner grant
// cannot be removed.
func (s APIServer) DeleteGrantByCollectionGrant(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, grantId api.GrantIdPath, _ api.DeleteGrantByCollectionGrantParams) {
	collID, ok := parseInt64Path(string(collectionId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	gid, ok := parseInt64Path(string(grantId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid grantId")
		return
	}
	if _, _, _, ok := s.authorizeCollection(w, r, collID, "stig-manager:collection", RoleOwner); !ok {
		return
	}
	if s.Grants == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if err := s.Grants.Delete(r.Context(), collID, gid); err != nil {
		s.handleGrantWriteErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers -------------------------------------------------------------

// storeToAPIGrant packs a store.Grant into the union api.Grant
// envelope using its user-shaped variant. UserGroup-shaped grants land
// in a later milestone.
func storeToAPIGrant(row store.Grant) (api.Grant, error) {
	gidStr := api.GrantId(strconv.FormatInt(row.GrantID, 10))
	uid := api.UserId(strconv.FormatInt(row.UserID, 10))
	user := api.UserBasicWithDisplayName{
		UserId:   uid,
		Username: api.Username(row.Username),
	}
	if row.DisplayName != "" {
		dn := api.DisplayName(row.DisplayName)
		user.DisplayName = &dn
	}
	proj := api.UserGrantProjected{
		GrantId: &gidStr,
		RoleId:  api.RoleId(row.RoleID),
		User:    user,
	}
	var out api.Grant
	if err := out.FromUserGrantProjected(proj); err != nil {
		return api.Grant{}, err
	}
	return out, nil
}

func (s APIServer) handleGrantWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeAuthError(w, http.StatusBadRequest, "grant conflict (duplicate or last owner)")
	case errors.Is(err, store.ErrNotFound):
		writeAuthError(w, http.StatusNotFound, "grant not found")
	default:
		s.logErr(r, "write grant", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to write grant")
	}
}
