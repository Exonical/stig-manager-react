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

// GetUserGroups — GET /user-groups.
func (s APIServer) GetUserGroups(w http.ResponseWriter, r *http.Request, params api.GetUserGroupsParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user:read") {
		return
	}
	if s.UserGroups == nil {
		writeJSON(w, http.StatusOK, []api.UserGroupProjected{})
		return
	}
	rows, err := s.UserGroups.List(r.Context())
	if err != nil {
		s.logErr(r, "list user groups", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list user groups")
		return
	}
	proj := userGroupProjection(params.Projection)
	out := make([]api.UserGroupProjected, 0, len(rows))
	for _, g := range rows {
		if proj.users || proj.collectionGrants {
			full, err := s.UserGroups.Get(r.Context(), g.UserGroupID)
			if err != nil {
				s.logErr(r, "user group projection", err)
				writeAuthError(w, http.StatusInternalServerError, "failed to project user group")
				return
			}
			g = full
		}
		out = append(out, toAPIUserGroup(g, proj))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateUserGroup — POST /user-groups.
func (s APIServer) CreateUserGroup(w http.ResponseWriter, r *http.Request, params api.CreateUserGroupParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.UserGroups == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var post api.UserGroupPostOrPut
	if err := json.Unmarshal(body, &post); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if post.Name == "" {
		writeAuthError(w, http.StatusBadRequest, "name required")
		return
	}
	desc := ""
	if post.Description != nil {
		desc = string(*post.Description)
	}
	userIDs, err := apiToStoreUserIDs(post.UserIds)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	grants, err := apiCollectionGrantsToStore(post.CollectionGrants)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.UserGroups.Create(r.Context(), store.UserGroupCreate{
		Name:             string(post.Name),
		Description:      desc,
		UserIDs:          userIDs,
		CollectionGrants: grants,
	})
	if err != nil {
		s.handleUserGroupWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIUserGroup(created, userGroupProjection(params.Projection)))
}

// GetUserGroup — GET /user-groups/{userGroupId}.
func (s APIServer) GetUserGroup(w http.ResponseWriter, r *http.Request, userGroupId api.UserGroupIdPath, params api.GetUserGroupParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user:read") {
		return
	}
	if s.UserGroups == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	gid, ok := parseInt64Path(string(userGroupId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userGroupId")
		return
	}
	g, err := s.UserGroups.Get(r.Context(), gid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "user group not found")
			return
		}
		s.logErr(r, "get user group", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch user group")
		return
	}
	writeJSON(w, http.StatusOK, toAPIUserGroup(g, userGroupProjection(params.Projection)))
}

// PatchUserGroup — PATCH /user-groups/{userGroupId}.
func (s APIServer) PatchUserGroup(w http.ResponseWriter, r *http.Request, userGroupId api.UserGroupIdPath, params api.PatchUserGroupParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.UserGroups == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	gid, ok := parseInt64Path(string(userGroupId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userGroupId")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var patch api.UserGroupPatch
	if err := json.Unmarshal(body, &patch); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p, err := apiToStoreUserGroupPatch(patch)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.UserGroups.Patch(r.Context(), gid, p)
	if err != nil {
		s.handleUserGroupWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUserGroup(updated, userGroupProjection(params.Projection)))
}

// PutUserGroup — PUT /user-groups/{userGroupId}.
func (s APIServer) PutUserGroup(w http.ResponseWriter, r *http.Request, userGroupId api.UserGroupIdPath, params api.PutUserGroupParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.UserGroups == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	gid, ok := parseInt64Path(string(userGroupId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userGroupId")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var put api.UserGroupPostOrPut
	if err := json.Unmarshal(body, &put); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if put.Name == "" {
		writeAuthError(w, http.StatusBadRequest, "name required")
		return
	}
	desc := ""
	if put.Description != nil {
		desc = string(*put.Description)
	}
	userIDs, err := apiToStoreUserIDs(put.UserIds)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	grants, err := apiCollectionGrantsToStore(put.CollectionGrants)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := string(put.Name)
	userIDsPtr := &userIDs
	grantsPtr := &grants
	descPtr := &desc
	p := store.UserGroupPatch{
		Name:             &name,
		Description:      descPtr,
		UserIDs:          userIDsPtr,
		CollectionGrants: grantsPtr,
	}
	updated, err := s.UserGroups.Patch(r.Context(), gid, p)
	if err != nil {
		s.handleUserGroupWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUserGroup(updated, userGroupProjection(params.Projection)))
}

// DeleteUserGroup — DELETE /user-groups/{userGroupId}.
func (s APIServer) DeleteUserGroup(w http.ResponseWriter, r *http.Request, userGroupId api.UserGroupIdPath, params api.DeleteUserGroupParams) {
	if !s.requireUsersScope(w, r, "stig-manager:user") {
		return
	}
	if s.UserGroups == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	gid, ok := parseInt64Path(string(userGroupId))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid userGroupId")
		return
	}
	row, err := s.UserGroups.Get(r.Context(), gid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "user group not found")
			return
		}
		s.logErr(r, "lookup user group", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch user group")
		return
	}
	if err := s.UserGroups.Delete(r.Context(), gid); err != nil {
		s.handleUserGroupWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUserGroup(row, userGroupProjection(params.Projection)))
}

func (s APIServer) handleUserGroupWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeAuthError(w, http.StatusNotFound, "user group not found")
	case errors.Is(err, store.ErrConflict):
		writeAuthError(w, http.StatusConflict, err.Error())
	default:
		s.logErr(r, "user group write", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to persist user group")
	}
}

type userGroupProjectionFlags struct {
	users            bool
	collectionGrants bool
}

func userGroupProjection(q *api.UserGroupProjectionQuery) userGroupProjectionFlags {
	if q == nil {
		return userGroupProjectionFlags{}
	}
	var f userGroupProjectionFlags
	for _, p := range *q {
		switch p {
		case "users":
			f.users = true
		case "collectionGrants", "collections":
			f.collectionGrants = true
		}
	}
	return f
}

func apiCollectionGrantsToStore(in *[]api.CollectionGrant) ([]store.GrantCreate, error) {
	if in == nil {
		return nil, nil
	}
	return apiToStoreGrants(*in)
}

func apiToStoreUserGroupPatch(in api.UserGroupPatch) (store.UserGroupPatch, error) {
	var out store.UserGroupPatch
	if in.Name != nil {
		s := string(*in.Name)
		out.Name = &s
	}
	if in.Description != nil {
		s := string(*in.Description)
		out.Description = &s
	}
	if in.UserIds != nil {
		ids, err := apiToStoreUserIDs(in.UserIds)
		if err != nil {
			return store.UserGroupPatch{}, err
		}
		out.UserIDs = &ids
	}
	if in.CollectionGrants != nil {
		grants, err := apiToStoreGrants(*in.CollectionGrants)
		if err != nil {
			return store.UserGroupPatch{}, err
		}
		out.CollectionGrants = &grants
	}
	return out, nil
}

func toAPIUserGroup(g store.UserGroup, proj userGroupProjectionFlags) api.UserGroupProjected {
	out := api.UserGroupProjected{
		UserGroupId: api.UserGroupId(strconv.FormatInt(g.UserGroupID, 10)),
		Name:        api.String255(g.Name),
	}
	if g.Description != "" {
		desc := api.String255Nullable(g.Description)
		out.Description = &desc
	}
	if proj.users {
		users := make([]api.UserBasicWithDisplayName, 0, len(g.Users))
		for _, u := range g.Users {
			users = append(users, api.UserBasicWithDisplayName{
				UserId:      api.UserId(strconv.FormatInt(u.UserID, 10)),
				Username:    api.Username(u.Username),
				DisplayName: nullableDisplayName(u.DisplayName),
			})
		}
		out.Users = &users
	}
	if proj.collectionGrants {
		grants := make([]api.UserGroupCollectionGrant, 0, len(g.CollectionGrants))
		for _, cg := range g.CollectionGrants {
			rid := api.RoleId(cg.RoleID)
			cid := api.CollectionId(strconv.FormatInt(cg.CollectionID, 10))
			name := api.CollectionName(cg.CollectionName)
			ug := api.UserGroupCollectionGrant{
				RoleId: &rid,
			}
			ug.Collection = &struct {
				CollectionId *api.CollectionId   `json:"collectionId,omitempty"`
				Name         *api.CollectionName `json:"name,omitempty"`
			}{CollectionId: &cid, Name: &name}
			grants = append(grants, ug)
		}
		out.CollectionGrants = &grants
	}
	return out
}

func nullableDisplayName(s string) *api.DisplayName {
	if s == "" {
		return nil
	}
	dn := api.DisplayName(s)
	return &dn
}
