package services

func AccountDeletionConfirmed(value string) bool {
	return value == "DELETE"
}
