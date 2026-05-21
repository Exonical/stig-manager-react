package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Exonical/stig-manager-react/api/internal/api"
	"github.com/Exonical/stig-manager-react/api/internal/store"
)

// requireJobsScope is the common precondition for the /jobs endpoints.
// Returns false and writes the error when the user is missing the
// requested scope. The OpenAPI spec gates these endpoints behind a
// single "stig-manager:op" scope (and :read for getters); we honour
// that.
func (s APIServer) requireJobsScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	return s.requiredScope(w, r, scope)
}

// jobsAvailable returns true when the database-backed JobRepo is
// wired. Falls back to a 503 when nil (matches the convention used by
// the other slices).
func (s APIServer) jobsAvailable(w http.ResponseWriter) bool {
	if s.Jobs == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "database unavailable")
		return false
	}
	return true
}

// GetJobs — GET /jobs.
func (s APIServer) GetJobs(w http.ResponseWriter, r *http.Request, _ api.GetJobsParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op:read") {
		return
	}
	if s.Jobs == nil {
		writeJSON(w, http.StatusOK, []api.Job{})
		return
	}
	rows, err := s.Jobs.List(r.Context())
	if err != nil {
		s.logErr(r, "list jobs", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	out := make([]api.Job, 0, len(rows))
	for _, j := range rows {
		out = append(out, toAPIJob(j))
	}
	writeJSON(w, http.StatusOK, out)
}

// PostJob — POST /jobs.
func (s APIServer) PostJob(w http.ResponseWriter, r *http.Request, _ api.PostJobParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var post api.JobCreate
	if err := json.Unmarshal(body, &post); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if string(post.Name) == "" {
		writeAuthError(w, http.StatusBadRequest, "name required")
		return
	}
	in := store.JobCreate{
		Name:        string(post.Name),
		Description: stringFromNullable(post.Description),
		TaskIDs:     apiTaskIDsToStore(post.Tasks),
	}
	if uid := userIDFromContext(r); uid != nil {
		in.CreatedByUserID = uid
	}
	applyAPIEventCreate(&in, post.Event)

	job, err := s.Jobs.Create(r.Context(), in)
	if err != nil {
		s.handleJobWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIJob(job))
}

// GetJob — GET /jobs/{jobId}.
func (s APIServer) GetJob(w http.ResponseWriter, r *http.Request, jobIDPath api.JobIdPath, _ api.GetJobParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op:read") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	jobID, err := strconv.ParseInt(string(jobIDPath), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid jobId")
		return
	}
	job, err := s.Jobs.Get(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "job not found")
			return
		}
		s.logErr(r, "get job", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch job")
		return
	}
	writeJSON(w, http.StatusOK, toAPIJob(job))
}

// PatchJob — PATCH /jobs/{jobId}.
func (s APIServer) PatchJob(w http.ResponseWriter, r *http.Request, jobIDPath api.JobIdPath, _ api.PatchJobParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	jobID, err := strconv.ParseInt(string(jobIDPath), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid jobId")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var patch api.JobUpdate
	if err := json.Unmarshal(body, &patch); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	in := store.JobPatch{}
	if patch.Name != nil {
		v := string(*patch.Name)
		in.Name = &v
	}
	if patch.Description != nil {
		in.Description = stringFromNullable(patch.Description)
	}
	if patch.Tasks != nil {
		ids := apiTaskIDsToStore(*patch.Tasks)
		in.TaskIDs = &ids
	}
	applyAPIEventPatch(&in, patch.Event)
	if uid := userIDFromContext(r); uid != nil {
		in.UpdatedByUserID = uid
	}
	job, err := s.Jobs.Patch(r.Context(), jobID, in)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "job not found")
			return
		}
		s.handleJobWriteErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIJob(job))
}

// DeleteJob — DELETE /jobs/{jobId}.
func (s APIServer) DeleteJob(w http.ResponseWriter, r *http.Request, jobIDPath api.JobIdPath, _ api.DeleteJobParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	jobID, err := strconv.ParseInt(string(jobIDPath), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid jobId")
		return
	}
	if err := s.Jobs.Delete(r.Context(), jobID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "job not found")
			return
		}
		s.logErr(r, "delete job", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to delete job")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAllTasks — GET /jobs/tasks.
func (s APIServer) GetAllTasks(w http.ResponseWriter, r *http.Request, _ api.GetAllTasksParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op:read") {
		return
	}
	if s.Jobs == nil {
		writeJSON(w, http.StatusOK, []api.JobTask{})
		return
	}
	rows, err := s.Jobs.ListTasks(r.Context())
	if err != nil {
		s.logErr(r, "list tasks", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	out := make([]api.JobTask, 0, len(rows))
	for _, t := range rows {
		out = append(out, toAPITask(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetRunsByJob — GET /jobs/{jobId}/runs.
func (s APIServer) GetRunsByJob(w http.ResponseWriter, r *http.Request, jobIDPath api.JobIdPath, _ api.GetRunsByJobParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op:read") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	jobID, err := strconv.ParseInt(string(jobIDPath), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid jobId")
		return
	}
	runs, err := s.Jobs.ListRuns(r.Context(), jobID)
	if err != nil {
		s.logErr(r, "list runs", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	out := make([]api.JobRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, toAPIRun(r))
	}
	writeJSON(w, http.StatusOK, out)
}

// RunImmediateJob — POST /jobs/{jobId}/runs.
func (s APIServer) RunImmediateJob(w http.ResponseWriter, r *http.Request, jobIDPath api.JobIdPath, _ api.RunImmediateJobParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	if s.JobRunner == nil {
		writeAuthError(w, http.StatusServiceUnavailable, "job runner unavailable")
		return
	}
	jobID, err := strconv.ParseInt(string(jobIDPath), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid jobId")
		return
	}
	if _, err := s.Jobs.Get(r.Context(), jobID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "job not found")
			return
		}
		s.logErr(r, "run immediate: load job", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to load job")
		return
	}
	runID, err := s.Jobs.CreateRun(r.Context(), jobID)
	if err != nil {
		s.logErr(r, "run immediate: create run", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to create run")
		return
	}
	if s.SynchronousRuns {
		s.JobRunner.RunSync(r.Context(), jobID, runID)
	} else {
		s.JobRunner.Start(r.Context(), jobID, runID)
	}
	writeJSON(w, http.StatusAccepted, api.JobRunCreated{RunId: runID.String()})
}

// GetRunById — GET /jobs/runs/{runId}.
func (s APIServer) GetRunById(w http.ResponseWriter, r *http.Request, runIDPath api.JobRunIdPath, _ api.GetRunByIdParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op:read") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	runID, err := uuid.Parse(string(runIDPath))
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid runId")
		return
	}
	run, err := s.Jobs.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "run not found")
			return
		}
		s.logErr(r, "get run", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch run")
		return
	}
	writeJSON(w, http.StatusOK, toAPIRun(run))
}

// DeleteRunById — DELETE /jobs/runs/{runId}.
func (s APIServer) DeleteRunById(w http.ResponseWriter, r *http.Request, runIDPath api.JobRunIdPath, _ api.DeleteRunByIdParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	runID, err := uuid.Parse(string(runIDPath))
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid runId")
		return
	}
	if err := s.Jobs.DeleteRun(r.Context(), runID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "run not found")
			return
		}
		s.logErr(r, "delete run", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to delete run")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetOutputByRun — GET /jobs/runs/{runId}/output.
func (s APIServer) GetOutputByRun(w http.ResponseWriter, r *http.Request, runIDPath api.JobRunIdPath, params api.GetOutputByRunParams) {
	if !s.requireJobsScope(w, r, "stig-manager:op:read") {
		return
	}
	if !s.jobsAvailable(w) {
		return
	}
	runID, err := uuid.Parse(string(runIDPath))
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid runId")
		return
	}
	if _, err := s.Jobs.GetRun(r.Context(), runID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "run not found")
			return
		}
		s.logErr(r, "get run for output", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to fetch run")
		return
	}
	after := int64(0)
	if params.AfterSeq != nil {
		after = int64(*params.AfterSeq)
	}
	rows, err := s.Jobs.ListOutput(r.Context(), runID, after)
	if err != nil {
		s.logErr(r, "list output", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to list output")
		return
	}
	out := make([]api.JobRunOutput, 0, len(rows))
	for _, o := range rows {
		out = append(out, toAPIOutput(o))
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- conversions / helpers ---------------------------------------

func toAPIJob(j store.Job) api.Job {
	out := api.Job{
		JobId: strconv.FormatInt(j.JobID, 10),
		Name:  api.String45(j.Name),
		Tasks: make(api.JobTaskList, 0, len(j.Tasks)),
		Created: j.CreatedAt,
	}
	if j.Description != nil {
		s := api.String255Nullable(*j.Description)
		out.Description = &s
	}
	for _, t := range j.Tasks {
		out.Tasks = append(out.Tasks, toAPITask(t))
	}
	rc := j.RunCount
	out.RunCount = &rc
	if j.LastRun != nil {
		lr := toAPIRun(*j.LastRun)
		out.LastRun = &lr
	}
	if !j.UpdatedAt.IsZero() {
		upd := api.StringDateTimeNullable(j.UpdatedAt)
		out.Updated = &upd
	}
	if ev, ok := buildAPIEvent(j); ok {
		out.Event = &ev
	}
	return out
}

func toAPITask(t store.Task) api.JobTask {
	out := api.JobTask{
		TaskId: strconv.FormatInt(t.TaskID, 10),
		Name:   api.String45(t.Name),
	}
	if t.Description != nil {
		desc := api.String255Nullable(*t.Description)
		out.Description = &desc
	}
	if t.Command != nil {
		cmd := api.String255(*t.Command)
		out.Command = &cmd
	}
	return out
}

func toAPIRun(r store.Run) api.JobRun {
	jobIDStr := strconv.FormatInt(r.JobID, 10)
	state := api.JobRunState(r.State)
	out := api.JobRun{
		RunId:   r.RunID.String(),
		JobId:   &jobIDStr,
		State:   &state,
		Created: r.CreatedAt,
	}
	if !r.UpdatedAt.IsZero() {
		upd := api.StringDateTimeNullable(r.UpdatedAt)
		out.Updated = &upd
	}
	return out
}

func toAPIOutput(o store.RunOutput) api.JobRunOutput {
	out := api.JobRunOutput{
		Ts:      o.Ts,
		Message: o.Message,
		Task:    api.String45(o.Task),
		Type:    o.Type,
	}
	if o.TaskID != nil {
		tid := strconv.FormatInt(*o.TaskID, 10)
		out.TaskId = &tid
	}
	return out
}

func apiTaskIDsToStore(in api.JobTaskListCreate) []int64 {
	out := make([]int64, 0, len(in))
	for _, raw := range in {
		id, err := strconv.ParseInt(string(raw), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

func stringFromNullable(n *api.String255Nullable) *string {
	if n == nil {
		return nil
	}
	s := string(*n)
	return &s
}

// applyAPIEventCreate copies an api.JobEventCreate union into the
// store.JobCreate fields.
func applyAPIEventCreate(in *store.JobCreate, ev *api.JobEventCreate) {
	if ev == nil {
		return
	}
	once, errOnce := ev.AsJobEventOnceCreate()
	if errOnce == nil && once.Type == api.JobEventOnceCreateType("once") {
		t := "once"
		in.EventType = &t
		starts := time.Time(once.Starts)
		in.EventStarts = &starts
		return
	}
	rec, errRec := ev.AsJobEventRecurringCreate()
	if errRec == nil && rec.Type == api.JobEventRecurringCreateType("recurring") {
		t := "recurring"
		in.EventType = &t
		if rec.Starts != nil {
			starts := time.Time(*rec.Starts)
			in.EventStarts = &starts
		}
		if rec.Ends != nil {
			ends := time.Time(*rec.Ends)
			in.EventEnds = &ends
		}
		if rec.Enabled != nil {
			in.EventEnabled = rec.Enabled
		}
		field := string(rec.Interval.Field)
		value := rec.Interval.Value
		in.EventIntervalFld = &field
		in.EventIntervalVal = &value
	}
}

// applyAPIEventPatch copies an api.JobEventCreate union into the
// store.JobPatch fields.
func applyAPIEventPatch(in *store.JobPatch, ev *api.JobEventCreate) {
	if ev == nil {
		return
	}
	once, errOnce := ev.AsJobEventOnceCreate()
	if errOnce == nil && once.Type == api.JobEventOnceCreateType("once") {
		t := "once"
		in.EventType = &t
		starts := time.Time(once.Starts)
		in.EventStarts = &starts
		return
	}
	rec, errRec := ev.AsJobEventRecurringCreate()
	if errRec == nil && rec.Type == api.JobEventRecurringCreateType("recurring") {
		t := "recurring"
		in.EventType = &t
		if rec.Starts != nil {
			starts := time.Time(*rec.Starts)
			in.EventStarts = &starts
		}
		if rec.Ends != nil {
			ends := time.Time(*rec.Ends)
			in.EventEnds = &ends
		}
		if rec.Enabled != nil {
			in.EventEnabled = rec.Enabled
		}
		field := string(rec.Interval.Field)
		value := rec.Interval.Value
		in.EventIntervalFld = &field
		in.EventIntervalVal = &value
	}
}

// buildAPIEvent returns the JobEvent union representing j's event_*
// columns.  ok=false when the job has no event configured.
func buildAPIEvent(j store.Job) (api.JobEvent, bool) {
	var out api.JobEvent
	if j.EventType == nil {
		return out, false
	}
	switch *j.EventType {
	case "once":
		var starts time.Time
		if j.EventStarts != nil {
			starts = *j.EventStarts
		}
		eid := api.String45(stringDeref(j.EventID))
		ev := api.JobEventOnce{
			EventId: eid,
			Type:    api.JobEventOnceTypeOnce,
			Starts:  starts,
			Enabled: &j.EventEnabled,
		}
		if err := out.FromJobEventOnce(ev); err != nil {
			return api.JobEvent{}, false
		}
	case "recurring":
		field := api.JobIntervalField(stringDeref(j.EventIntervalFld))
		ev := api.JobEventRecurring{
			EventId: api.String45(stringDeref(j.EventID)),
			Type:    api.JobEventRecurringTypeRecurring,
			Enabled: &j.EventEnabled,
			Interval: api.JobInterval{
				Field: field,
				Value: stringDeref(j.EventIntervalVal),
			},
		}
		if j.EventStarts != nil {
			st := api.StringDateTimeNullable(*j.EventStarts)
			ev.Starts = &st
		}
		if j.EventEnds != nil {
			en := api.StringDateTimeNullable(*j.EventEnds)
			ev.Ends = &en
		}
		if err := out.FromJobEventRecurring(ev); err != nil {
			return api.JobEvent{}, false
		}
	default:
		return out, false
	}
	return out, true
}

func stringDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// userIDFromContext extracts the app_user.user_id of the current
// principal (when present in context). Returns nil when no user is
// attached or when the user has no upserted DB row yet.
func userIDFromContext(r *http.Request) *int64 {
	// The current auth.User payload does not expose a numeric user_id;
	// upserts happen lazily on first login and aren't required for the
	// jobs path.  Return nil so the created_by_user_id column is NULL.
	return nil
}

func (s APIServer) handleJobWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	s.logErr(r, "job write", err)
	writeAuthError(w, http.StatusInternalServerError, "failed to write job")
}
