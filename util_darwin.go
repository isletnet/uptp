package uptp

import "strings"

func getOsName() (osName string) {
	output := execOutput("sw_vers", "-productVersion")
	osName = "Mac OS X " + strings.TrimSpace(output)
	return
}
