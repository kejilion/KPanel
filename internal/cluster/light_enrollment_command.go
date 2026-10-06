package cluster

func (s *Service) lightEnrollmentCommand(token, name string) (string, error) {
	command := "bash <(curl -fsSL https://kejilion.sh) kpanel node join '" + token + "'"
	if name != "" {
		command += " --name " + shellSingleQuote(name)
	}
	return command, nil
}
