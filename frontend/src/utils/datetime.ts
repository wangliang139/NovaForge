export function getTimeSeconds() {
  return Math.floor(Date.now() / 1000);
}

export function getTimeMills() {
  return Date.now();
}
