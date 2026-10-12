export async function api(path, method = "GET", body) {
  const response = await fetch("/api" + path, {
    method,
    credentials: "same-origin",
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error("服务暂不可用，请稍后重试");
  }
  if (!response.ok) {
    const e = new Error(data.error || "请求失败");
    e.status = response.status;
    throw e;
  }
  return data;
}
