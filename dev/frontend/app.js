const apiBaseInput = document.querySelector("#api-base");
const form = document.querySelector("#contact-form");
const responseBox = document.querySelector("#response");
const statusBadge = document.querySelector("#status-badge");
const responseTime = document.querySelector("#response-time");
const schemaState = document.querySelector("#schema-state");

let fields = [];
let apiBasePath = "/api/v1";
let sendInFlightKey = "";

const setDefaultAPIBase = () => {
  if (apiBaseInput.value) {
    return;
  }
  const port = apiBaseInput.dataset.apiPort || "8080";
  const contextPath = apiBaseInput.dataset.contextPath || "";
  apiBaseInput.value = `${window.location.protocol}//${window.location.hostname}:${port}${contextPath}`;
};

const apiBase = () => apiBaseInput.value.replace(/\/$/, "");

const apiPath = (path) => {
  const baseURL = new URL(apiBase());
  const basePath = baseURL.pathname.replace(/\/$/, "");
  return basePath ? `/api/v1${path}` : `${apiBasePath}${path}`;
};

const fieldElement = (field) => {
  const id = `field-${field.name}`;
  const wrapper = document.createElement("label");
  wrapper.className = "field";
  wrapper.htmlFor = id;

  const caption = document.createElement("span");
  caption.textContent = field.required ? `${field.label} *` : field.label;
  wrapper.append(caption);

  let input;
  if (field.type === "textarea") {
    input = document.createElement("textarea");
    input.rows = 6;
  } else if (field.type === "select") {
    input = document.createElement("select");
    for (const option of field.options || []) {
      const item = document.createElement("option");
      item.value = option;
      item.textContent = option;
      input.append(item);
    }
  } else {
    input = document.createElement("input");
    input.type = field.type || "text";
  }

  input.id = id;
  input.name = field.name;
  input.placeholder = field.placeholder || "";
  input.value = field.default || "";
  input.required = Boolean(field.required);
  if (field.maxLength > 0) {
    input.maxLength = field.maxLength;
  }
  wrapper.append(input);

  if (field.help) {
    const help = document.createElement("small");
    help.textContent = field.help;
    wrapper.append(help);
  }

  return wrapper;
};

const renderForm = () => {
  form.replaceChildren();
  for (const field of fields) {
    form.append(fieldElement(field));
  }
};

const payload = () => {
  const data = {};
  for (const field of fields) {
    const element = form.elements[field.name];
    data[field.name] = element ? element.value : "";
  }
  return data;
};

const setResponse = (status, body) => {
  const receivedAt = new Date();
  statusBadge.textContent = status;
  responseTime.textContent = `Received ${receivedAt.toLocaleString(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
  })}`;
  responseBox.textContent = JSON.stringify(body, null, 2);
};

const newIdempotencyKey = () => {
  if (globalThis.crypto && typeof globalThis.crypto.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
};

const request = async (path) => {
  const headers = { "Content-Type": "application/json" };
  if (path === "/sendmail") {
    if (!sendInFlightKey) {
      sendInFlightKey = newIdempotencyKey();
    }
    headers["Idempotency-Key"] = sendInFlightKey;
  }
  try {
    const res = await fetch(`${apiBase()}${apiPath(path)}`, {
      method: "POST",
      headers,
      body: JSON.stringify(payload()),
    });
    const body = await res.json().catch(() => ({}));
    setResponse(`${res.status} ${res.statusText}`, body);
  } finally {
    if (path === "/sendmail") {
      sendInFlightKey = "";
    }
  }
};

const loadSchema = async () => {
  schemaState.textContent = "Loading form schema";
  const res = await fetch(`${apiBase()}${apiPath("/schema")}`);
  const schema = await res.json();
  fields = schema.fields || [];
  apiBasePath = schema.apiBasePath || "/api/v1";
  renderForm();
  schemaState.textContent = `${fields.length} fields from backend config`;
  setResponse(`${res.status} ${res.statusText}`, schema);
};

document.querySelector("#validate").addEventListener("click", () => request("/validate"));
document.querySelector("#send").addEventListener("click", () => request("/sendmail"));
apiBaseInput.addEventListener("change", loadSchema);

setDefaultAPIBase();
loadSchema().catch((error) => {
  schemaState.textContent = "Schema load failed";
  setResponse("Error", { error: error.message });
});
