package uptp

import "golang.org/x/sys/windows/registry"

func getOsName() (osName string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return
	}
	defer k.Close()
	pn, _, err := k.GetStringValue("ProductName")
	if err == nil {
		osName = pn
	}
	return
}
