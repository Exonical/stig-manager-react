package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/auth"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// requireUsersScope is the common precondition for the /users and
// /user-groups endpoints: an authenticated principal with the
// `stig-manager:user` (or :read) scope. Returns false and writes the
// error when the precondition fails.
func (s APIServer) requireUsersScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	user, ok := auth.FromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if !user.HasScope(scope) {
		writeAuthError(w, http.StatusForbidden, "missing required scope: "+scope)
		return false
	}
	return true
}

// GetUsers — GET /users.
func (s APIServer) GetUsers(w http.ResponseWriter, r *http.Request, params api.GetUsersParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user:read") {
		return
	}
	if s.Users == nil {
		writeJSON(w, http.StatusOK, []api.UserProjected{})
		return
	}
	filter := store.UserListFilter{}
	if params.Username != nil {
		filter.Username = string(*params.Username)
	}
	if params.UsernameMatch != nil {
		filter.UsernameMatch = string(*params.UsernameMatch)
	}
	if params.Status != nil {
		filter.Status = string(*params.Status)
	}
	if params.Privilege != nil {
		filter.Privilege = string(*params.Privilege)
	}
	rows, err := s.Users.ListAdmin(r.Context(), filter)
	if err != nil {
		s.logErr(r, "list users", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	out := make([]api.UserProjected, 0, len(rows))
	projection := userProjection(params.Projection)
	for _, u := range rows {
		// list view skips heavy projections unless requested
		if projection.collectionGrants || projection.userGroups {
			if err := s.Users.AttachExtras(r.Context(), &u); err != nil {
				s.logErr(r, "user projection", err)
				writeAuthError(w, http.StatusInternalServerError, "failed to project user")
				return
			}
		}
		out = append(out, toAPIUser(u, projection))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateUser — POST /users.
func (s APIServer) CreateUser(w http.ResponseWriter, r *http.Request, params api.CreateUserParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var post api.UserPost
	if err := json.Unmarshal(body, &post); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(string(post.Username)) == "" {
		writeAuthError(w, http.StatusBadRequest, "username required")
		return
	}
	grants, err := apiToStoreGrants(post.CollectionGrants)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	groupIDs, err := apiToStoreUserGroupIDs(post.UserGroups)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	in := store.UserCreate{
		Username:         string(post.Username),
		Status:           "available",
		CollectionGrants: grants,
		UserGroups:       groupIDs,
	}
	created, err := s.Users.CreateAdmin(r.Context(), in)
	if err != nil {
		s.handleUserWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIUser(created, userProjection(params.Projection)))
}

// GetUser — GET /user. Returns the authenticated principal's app_user
// row, upserting first so first-time visitors get a row immediately.
//
// Deliberately does NOT enforce the `stig-manager:user:read` scope
// recommended by the upstream OpenAPI: a user reading their own
// record is not an admin operation. Without this concession every
// signed-in user would need :read just to see their own userId in the
// SPA, even though the OIDC token already carries the same identity
// claims.
func (s APIServer) GetUser(w http.ResponseWriter, r *http.Request, _ api.GetUserParams) {
	user, ok := auth.FromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if _, err := s.Users.Upsert(r.Context(),
		user.Subject, user.Username, user.Name, user.Email, user.Raw,
	); err != nil {
		s.logErr(r, "upsert current user", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to record user")
		return
	}
	appUser, err := s.Users.GetBySubject(r.Context(), user.Subject)
	if err != nil {
		s.logErr(r, "get current user by subject", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	row, err := s.Users.GetAdmin(r.Context(), appUser.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "user not found")
			return
		}
		s.logErr(r, "get current user", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	// Always project collectionGrants for /user — the SPA needs them to
	// know which role the user holds in each visible collection without
	// a second round-trip.
	writeJSON(w, http.StatusOK, toAPIUser(row, userProjectionFlags{collectionGrants: true, userGroups: true}))
}

// GetUserByUserId — GET /users/{userId}.
func (s APIServer) GetUserByUserId(w http.ResponseWriter, r *http.Request, userId api.UserIdPath, params api.GetUserByUserIdParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user:read") {
		return
	}
	if s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	uid, ok := parseInt64Path(string(userId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userId")
		return
	}
	row, err := s.Users.GetAdmin(r.Context(), uid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "user not found")
			return
		}
		s.logErr(r, "get user", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(row, userProjection(params.Projection)))
}

// UpdateUser — PATCH /users/{userId}.
func (s APIServer) UpdateUser(w http.ResponseWriter, r *http.Request, userId api.UserIdPath, params api.UpdateUserParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	uid, ok := parseInt64Path(string(userId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userId")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var patch api.UserPatch
	if err := json.Unmarshal(body, &patch); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p, err := apiToStoreUserPatch(patch)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.Users.PatchAdmin(r.Context(), uid, p)
	if err != nil {
		s.handleUserWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(updated, userProjection(params.Projection)))
}

// ReplaceUser — PUT /users/{userId}.
func (s APIServer) ReplaceUser(w http.ResponseWriter, r *http.Request, userId api.UserIdPath, params api.ReplaceUserParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	uid, ok := parseInt64Path(string(userId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userId")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var put api.UserPut
	if err := json.Unmarshal(body, &put); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	grants, err := apiToStoreGrants(put.CollectionGrants)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	groupIDs, err := apiToStoreUserGroupIDs(put.UserGroups)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	username := string(put.Username)
	groupIDsPtr := &groupIDs
	grantsPtr := &grants
	usernamePtr := &username
	p := store.UserPatch{
		Username:         usernamePtr,
		UserGroups:       groupIDsPtr,
		CollectionGrants: grantsPtr,
	}
	if put.Status != nil {
		st := string(*put.Status)
		p.Status = &st
	}
	updated, err := s.Users.PatchAdmin(r.Context(), uid, p)
	if err != nil {
		s.handleUserWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(updated, userProjection(params.Projection)))
}

// DeleteUser — DELETE /users/{userId}.
func (s APIServer) DeleteUser(w http.ResponseWriter, r *http.Request, userId api.UserIdPath, params api.DeleteUserParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	uid, ok := parseInt64Path(string(userId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userId")
		return
	}
	row, err := s.Users.GetAdmin(r.Context(), uid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "user not found")
			return
		}
		s.logErr(r, "lookup user for delete", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	if err := s.Users.DeleteAdmin(r.Context(), uid); err != nil {
		s.handleUserWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(row, userProjection(params.Projection)))
}

// handleUserWriteErr maps store errors to HTTP statuses for the user
// CRUD endpoints.
func (s APIServer) handleUserWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeAuthError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, store.ErrConflict):
		writeAuthError(w, http.StatusConflict, err.Error())
	default:
		s.logErr(r, "user write", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to persist user")
	}
}

// userProjectionFlags tracks which optional User projections the
// caller requested via ?projection=.
type userProjectionFlags struct {
	collectionGrants bool
	userGroups       bool
}

func userProjection(q *api.UserProjectionQuery) userProjectionFlags {
	if q == nil {
		return userProjectionFlags{}
	}
	var f userProjectionFlags
	for _, p := range *q {
		switch p {
		case "collectionGrants":
			f.collectionGrants = true
		case "userGroups":
			f.userGroups = true
		}
	}
	return f
}

func apiToStoreGrants(in []api.CollectionGrant) ([]store.GrantCreate, error) {
	out := make([]store.GrantCreate, 0, len(in))
	for _, g := range in {
		cid, ok := parseInt64Path(string(g.CollectionId))
		if !ok {
			return nil, errors.New("invalid collectionId in collectionGrants")
		}
		role := int16(g.RoleId)
		if role < 1 || role > 4 {
			return nil, errors.New("collectionGrants[].roleId must be 1..4")
		}
		out = append(out, store.GrantCreate{
			CollectionID: cid,
			RoleID:       role,
		})
	}
	return out, nil
}

func apiToStoreUserGroupIDs(in *[]api.UserGroupId) ([]int64, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]int64, 0, len(*in))
	for _, id := range *in {
		gid, ok := parseInt64Path(string(id))
		if !ok {
			return nil, errors.New("invalid userGroupId in userGroups")
		}
		out = append(out, gid)
	}
	return out, nil
}

func apiToStoreUserIDs(in *[]api.UserId) ([]int64, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]int64, 0, len(*in))
	for _, id := range *in {
		uid, ok := parseInt64Path(string(id))
		if !ok {
			return nil, errors.New("invalid userId in userIds")
		}
		out = append(out, uid)
	}
	return out, nil
}

func apiToStoreUserPatch(in api.UserPatch) (store.UserPatch, error) {
	var out store.UserPatch
	if in.Username != nil {
		s := string(*in.Username)
		out.Username = &s
	}
	if in.Status != nil {
		s := string(*in.Status)
		out.Status = &s
	}
	if in.UserGroups != nil {
		ids, err := apiToStoreUserGroupIDs(in.UserGroups)
		if err != nil {
			return store.UserPatch{}, err
		}
		out.UserGroups = &ids
	}
	if in.CollectionGrants != nil {
		grants, err := apiToStoreGrants(*in.CollectionGrants)
		if err != nil {
			return store.UserPatch{}, err
		}
		out.CollectionGrants = &grants
	}
	return out, nil
}

func toAPIUser(u store.User, proj userProjectionFlags) api.UserProjected {
	out := api.UserProjected{
		UserId:   api.UserId(strconv.FormatInt(u.UserID, 10)),
		Username: api.Username(u.Username),
	}
	if u.DisplayName != "" {
		dn := api.DisplayName(u.DisplayName)
		out.DisplayName = &dn
	}
	if u.Email != "" {
		em := u.Email
		out.Email = &em
	}
	status := api.UserStatus(u.Status)
	out.Status = &status
	out.Privileges = &api.UserPrivileges{
		Admin:            &u.PrivilegeAdmin,
		CreateCollection: &u.PrivilegeCreateCollection,
	}
	if proj.collectionGrants {
		grants := make([]api.CollectionGrantProjected, 0, len(u.CollectionGrants))
		for _, g := range u.CollectionGrants {
			rid := api.RoleId(g.RoleID)
			gp := api.CollectionGrantProjected{
				RoleId: &rid,
			}
			cid := api.CollectionId(strconv.FormatInt(g.CollectionID, 10))
			name := api.CollectionName(g.CollectionName)
			gp.Collection = &struct {
				CollectionId *api.CollectionId   `json:"collectionId,omitempty"`
				Name         *api.CollectionName `json:"name,omitempty"`
			}{CollectionId: &cid, Name: &name}
			grants = append(grants, gp)
		}
		out.CollectionGrants = &grants
	}
	if proj.userGroups {
		groups := make([]api.UserGroupBasic, 0, len(u.UserGroups))
		for _, g := range u.UserGroups {
			groups = append(groups, api.UserGroupBasic{
				UserGroupId: api.UserGroupId(strconv.FormatInt(g.UserGroupID, 10)),
				Name:        api.String255(g.Name),
			})
		}
		out.UserGroups = &groups
	}
	return out
}
