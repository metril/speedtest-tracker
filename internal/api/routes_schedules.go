package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerSchedules(api huma.API) {
	const t = "schedules"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/schedules", ID: "listSchedules", Tag: t, Summary: "List schedules"}, d.listSchedules)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/schedules", ID: "createSchedule", Tag: t, Summary: "Create a schedule", Status: 201, Errors: []int{409}}, d.createSchedule)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/schedules/validate", ID: "validateCron", Tag: t, Summary: "Validate a cron expression and preview fire times"}, d.validateCron)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/schedules/{id}", ID: "getSchedule", Tag: t, Summary: "Get a schedule"}, d.getSchedule)
	register(api, route{Method: http.MethodPut, Path: "/api/v1/schedules/{id}", ID: "updateSchedule", Tag: t, Summary: "Replace a schedule", Errors: []int{409}}, d.updateSchedule)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/schedules/{id}", ID: "deleteSchedule", Tag: t, Summary: "Delete a schedule", Status: 204}, d.deleteSchedule)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/schedules/{id}/run", ID: "runSchedule", Tag: t, Summary: "Run a schedule's targets now", Status: 202, Errors: []int{503}}, d.runSchedule)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/schedules/{id}/next", ID: "getScheduleNext", Tag: t, Summary: "Upcoming fire times of a schedule"}, d.scheduleNext)
}
