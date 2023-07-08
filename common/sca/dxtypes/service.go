package dxtypes

// Service for describing service config, for systemctl, crontab, rc[\d]{,2}.d/ nginx/httpd/apache2 config
// and docker containers ports / fs mount and soon.
type Service struct {
	// contab / systemctl / nginx
	ApplicationName string

	// config / network / fs / command
	ServiceType    string
	ServiceName    string
	ServiceContent string
	ExtraInfo      []*InfoPair
}
