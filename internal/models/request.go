package models

type ClientContext struct {
	UserID          string
	DeviceID        string
	Platform        string
	PlatformVersion string
	DeviceModel     string
	Architecture    string
	DeviceBrand     string
	Mobile          string
	Device          string
	AppName         string
	AppVersion      string
	PackageName     string
	BuildNumber     string
}

func (c *ClientContext) GetTargetingKey() string {
	if c.UserID != "" {
		return c.UserID
	}
	if c.DeviceID != "" {
		return c.DeviceID
	}
	return "anonymous"
}
