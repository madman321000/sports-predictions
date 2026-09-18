const API = (
  import.meta.env.VITE_API_BASE_URL || "http://localhost:8080"
).replace(/\/$/, "");
export async function get(path, signal) {
  const response = await fetch(API + path, { signal });
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message.slice(0, 250) || `API returned ${response.status}`);
  }
  return response.json();
}
