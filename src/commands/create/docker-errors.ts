export function dockerUnavailableError(): Error {
  return new Error("Docker is not available. Install Docker and ensure the daemon is running.");
}
