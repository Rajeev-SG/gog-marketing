package runner

import (
	"net/http"
	"regexp"
)

var classroomCourseID = regexp.MustCompile(`^(?:(?:d|p):)?[A-Za-z0-9_-]{1,256}$`)

const (
	classroomOperationCreate = "create"
	classroomOperationGet    = "get"
)

func nativeClassroomCourseBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 5)

	for _, operation := range []struct{ name, method string }{{"list", http.MethodGet}, {classroomOperationGet, http.MethodGet}, {classroomOperationCreate, http.MethodPost}, {"update", http.MethodPatch}, {"delete", http.MethodDelete}} {
		scopes := []string{"classroom.courses"}
		if operation.method == http.MethodGet {
			scopes = append(scopes, "classroom.courses.readonly")
		}
		result = append(result, NativeToolBinding{Name: "classroom_courses_" + operation.name, Service: "classroom", Write: operation.method != http.MethodGet, destructive: operation.method == http.MethodDelete, hosts: []string{"classroom.googleapis.com"}, scopes: scopes, methods: classroomCourseMethods(operation.method), paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			path := "/v1/courses"

			if operation.name != "list" && operation.method != http.MethodPost {
				id, _ := args["course_id"].(string)
				if !classroomCourseID.MatchString(id) {
					return nil, invalidf("invalid Classroom course ID")
				}
				path += "/" + id
			}

			return []string{path}, nil
		}})
	}

	return result
}

func classroomCourseMethods(method string) []string {
	if method == http.MethodDelete || method == http.MethodPatch {
		return []string{http.MethodGet, method}
	}

	return []string{method}
}
