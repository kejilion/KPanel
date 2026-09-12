package agent

func (s *Server) backupHostWorkersActive() bool {
	if s.terminals != nil && s.terminals.Busy() {
		return true
	}
	if s.systemManager != nil && s.systemManager.DiskJobActive() {
		return true
	}
	active := func(status string) bool { return status == "queued" || status == "running" }
	if s.docker != nil {
		for _, j := range s.docker.MaintenanceJobs() {
			if active(j.Status) {
				return true
			}
		}
	}
	if s.appMarket != nil {
		for _, j := range s.appMarket.AppJobs() {
			if active(j.Status) {
				return true
			}
		}
	}
	if s.webEnvironment != nil {
		for _, j := range s.webEnvironment.Jobs() {
			if active(j.Status) {
				return true
			}
		}
	}
	if s.sitesManager != nil {
		for _, j := range s.sitesManager.InstallationJobs() {
			if active(j.Status) {
				return true
			}
		}
	}
	if s.files != nil {
		jobs, err := s.files.ArchiveJobs()
		if err != nil {
			return true
		}
		for _, j := range jobs {
			if active(j.State) {
				return true
			}
		}
	}
	return false
}
