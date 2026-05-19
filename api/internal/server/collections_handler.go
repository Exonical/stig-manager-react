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

// defaultCollectionSettings is the JSONB blob written when a client
// creates a Collection without an explicit settings body. It populates
// every required field of the api.CollectionSettings schema so reads
// produce a valid response without any post-hydration logic.
const defaultCollectionSettings = `{
  "fields": {
    "comment": {"enabled": "always", "required": "optional"},
    "detail":  {"enabled": "always", "required": "optional"}
  },
  "status": {
    "canAccept": true,
    "minAcceptGrant": 3,
    "resetCriteria": "result"
  },
  "history": {
    "maxReviews": 5
  }
}`

// GetCollections lists Collections visible to the requester. The
// `name`/`name-match` query parameters provide a basic substring filter;
// projections/metadata filters arrive in a later milestone.
func (s APIServer) GetCollections(w http.ResponseWriter, r *http.Request, params api.GetCollectionsParams) {
	if !s.requiredScope(w, r, "stig-manager:collection:read") {
		return
	}
	if s.Collections == nil {
		writeJSON(w, http.StatusOK, []api.Collection{})
		return
	}

	opts := store.ListCollectionsOptions{}
	if params.Name != nil {
		opts.NameContains = string(*params.Name)
	}

	rows, err := s.Collections.List(r.Context(), opts)
	if err != nil {
		s.logErr(r, "list collections", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list collections")
		return
	}
	out := make([]api.Collection, 0, len(rows))
	for _, row := range rows {
		c, err := storeToAPICollection(row)
		if err != nil {
			s.logErr(r, "decode collection", err)
			writeAuthError(w, http.StatusInternalServerError, "failed to decode collection")
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetCollection returns a single Collection by id.
func (s APIServer) GetCollection(w http.ResponseWriter, r *http.Request, collectionId api.CollectionIdPath, _ api.GetCollectionParams) {
	if !s.requiredScope(w, r, "stig-manager:collection:read") {
		return
	}
	id, err := strconv.ParseInt(string(collectionId), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid collectionId")
		return
	}
	if s.Collections == nil {
		writeAuthError(w, http.StatusNotFound, "collection not found")
		return
	}
	row, err := s.Collections.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "collection not found")
			return
		}
		s.logErr(r, "get collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to get collection")
		return
	}
	c, err := storeToAPICollection(row)
	if err != nil {
		s.logErr(r, "decode collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to decode collection")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// CreateCollection inserts a new Collection along with its grants. The
// requester is automatically upserted into app_user and given the
// owner role (1..4) only as part of the request body — there is no
// implicit grant added in this milestone.
func (s APIServer) CreateCollection(w http.ResponseWriter, r *http.Request, _ api.CreateCollectionParams) {
	if !s.requiredScope(w, r, "stig-manager:collection") {
		return
	}
	if s.Collections == nil || s.Users == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	user, ok := auth.FromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in api.CollectionCreateOrReplace
	if err := json.Unmarshal(body, &in); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(string(in.Name)) == "" {
		writeAuthError(w, http.StatusBadRequest, "name is required")
		return
	}
	grants, err := parseCreateGrants(body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(grants) == 0 {
		writeAuthError(w, http.StatusBadRequest, "at least one grant is required")
		return
	}

	// Ensure the requesting principal exists in app_user; their user_id
	// is not used here directly (the body's grants reference user ids
	// the caller chose) but the row is needed for later milestones'
	// creator-tracking and audit logging.
	if _, err := s.Users.Upsert(r.Context(),
		user.Subject, user.Username, user.Name, user.Email, user.Raw,
	); err != nil {
		s.logErr(r, "upsert current user", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to record user")
		return
	}

	settings := []byte(defaultCollectionSettings)
	if in.Settings != nil {
		raw, err := json.Marshal(in.Settings)
		if err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
			return
		}
		settings = raw
	}
	metadata := []byte(`{}`)
	if in.Metadata != nil {
		raw, err := json.Marshal(*in.Metadata)
		if err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid metadata: "+err.Error())
			return
		}
		metadata = raw
	}
	description := ""
	if in.Description != nil {
		description = string(*in.Description)
	}

	row, err := s.Collections.Create(r.Context(), store.CollectionCreate{
		Name:        string(in.Name),
		Description: description,
		Settings:    settings,
		Metadata:    metadata,
		Grants:      grants,
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicateName) {
			writeAuthError(w, http.StatusBadRequest, "duplicate collection name")
			return
		}
		s.logErr(r, "create collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to create collection")
		return
	}
	out, err := storeToAPICollection(row)
	if err != nil {
		s.logErr(r, "decode collection", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to decode collection")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// parseCreateGrants extracts user-shaped grants from the raw request
// body. We avoid using the generated union-helpers because in their
// current shape they expose `union` as an unexported field; the
// shape we accept is the documented OpenAPI schema, so peeking at
// the raw JSON is precise enough.
func parseCreateGrants(body []byte) ([]store.CollectionCreateGrant, error) {
	var envelope struct {
		Grants []struct {
			UserID      *string `json:"userId,omitempty"`
			UserGroupID *string `json:"userGroupId,omitempty"`
			RoleID      *int    `json:"roleId,omitempty"`
		} `json:"grants"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	out := make([]store.CollectionCreateGrant, 0, len(envelope.Grants))
	for i, g := range envelope.Grants {
		if g.RoleID == nil {
			return nil, errAtIndex(i, "roleId is required")
		}
		if *g.RoleID < 1 || *g.RoleID > 4 {
			return nil, errAtIndex(i, "roleId must be 1..4")
		}
		if g.UserGroupID != nil && *g.UserGroupID != "" {
			// User-group grants are intentionally not yet supported.
			return nil, errAtIndex(i, "userGroupId grants are not yet supported")
		}
		if g.UserID == nil || *g.UserID == "" {
			return nil, errAtIndex(i, "userId is required")
		}
		uid, err := strconv.ParseInt(*g.UserID, 10, 64)
		if err != nil {
			return nil, errAtIndex(i, "invalid userId")
		}
		out = append(out, store.CollectionCreateGrant{
			UserID: uid,
			RoleID: int16(*g.RoleID),
		})
	}
	return out, nil
}

func errAtIndex(i int, msg string) error {
	return errors.New("grants[" + strconv.Itoa(i) + "]: " + msg)
}

// storeToAPICollection projects a store.Collection (raw JSONB blobs +
// timestamps) onto the OpenAPI Collection type.
func storeToAPICollection(in store.Collection) (api.Collection, error) {
	out := api.Collection{
		CollectionId: api.CollectionId(strconv.FormatInt(in.CollectionID, 10)),
		Name:         api.CollectionName(in.Name),
		Created:      in.CreatedAt,
	}
	if in.Description != "" {
		desc := api.CollectionDescription(in.Description)
		out.Description = &desc
	}
	settings := []byte(in.Settings)
	if len(settings) == 0 || string(settings) == "{}" {
		settings = []byte(defaultCollectionSettings)
	}
	if err := json.Unmarshal(settings, &out.Settings); err != nil {
		return api.Collection{}, err
	}
	meta := api.Metadata{}
	if len(in.Metadata) > 0 {
		if err := json.Unmarshal(in.Metadata, &meta); err != nil {
			return api.Collection{}, err
		}
	}
	out.Metadata = meta
	return out, nil
}
