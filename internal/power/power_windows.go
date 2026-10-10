package power

// A full shutdown (no /hybrid), so network cards keep their Wake-on-LAN state.
func shutdownCmd() (string, []string) {
	return "shutdown.exe", []string{"/s", "/t", "5", "/c", "Shut down from Spout Remote Play", "/d", "p:0:0"}
}
