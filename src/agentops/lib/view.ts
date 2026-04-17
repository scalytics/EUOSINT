export function formatTime(value: string): string {
  if (!value) return "-";
  return value.replace("T", " ").replace("Z", " UTC");
}
