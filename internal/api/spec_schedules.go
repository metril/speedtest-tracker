package api

import "net/http"

// ScheduleList is the GET /schedules body.
type ScheduleList struct {
	Schedules []scheduleView `json:"schedules"`
}

// ScheduleSaved is the schedule create/update response.
type ScheduleSaved struct {
	Schedule scheduleView `json:"schedule"`
	Warnings []string     `json:"warnings" doc:"Overlap warnings computed for the saved schedule."`
}

// CronValidateRequest is the POST /schedules/validate body.
type CronValidateRequest struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
}

// CronValidateResponse previews the next fire times of a valid expression.
type CronValidateResponse struct {
	OK   bool     `json:"ok"`
	Next []string `json:"next"`
}

// ScheduleNext is the GET /schedules/{id}/next body.
type ScheduleNext struct {
	Next []string `json:"next"`
}

func init() { registerSpec(scheduleSpecs) }

func scheduleSpecs() []opSpec {
	const t = "schedules"
	id := []p{idParam("Schedule ID")}
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/schedules", ID: "listSchedules", Tag: t, Summary: "List schedules", Out: ScheduleList{}},
		{Method: http.MethodPost, Path: "/api/v1/schedules", ID: "createSchedule", Tag: t, Summary: "Create a schedule", Body: scheduleBody{}, Status: 201, Out: ScheduleSaved{}, Errors: []int{409}},
		{Method: http.MethodPost, Path: "/api/v1/schedules/validate", ID: "validateCron", Tag: t, Summary: "Validate a cron expression and preview fire times", Body: CronValidateRequest{}, Out: CronValidateResponse{}},
		{Method: http.MethodGet, Path: "/api/v1/schedules/{id}", ID: "getSchedule", Tag: t, Summary: "Get a schedule", Params: id, Out: scheduleView{}},
		{Method: http.MethodPut, Path: "/api/v1/schedules/{id}", ID: "updateSchedule", Tag: t, Summary: "Replace a schedule", Params: id, Body: scheduleBody{}, Out: ScheduleSaved{}, Errors: []int{409}},
		{Method: http.MethodDelete, Path: "/api/v1/schedules/{id}", ID: "deleteSchedule", Tag: t, Summary: "Delete a schedule", Params: id},
		{Method: http.MethodPost, Path: "/api/v1/schedules/{id}/run", ID: "runSchedule", Tag: t, Summary: "Run a schedule's targets now", Params: id, Status: 202, Out: RunQueued{}, Errors: []int{503}},
		{Method: http.MethodGet, Path: "/api/v1/schedules/{id}/next", ID: "getScheduleNext", Tag: t, Summary: "Upcoming fire times of a schedule", Params: id, Out: ScheduleNext{}},
	}
}
