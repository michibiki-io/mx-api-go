const apiBaseInput = document.querySelector("#api-base");
const form = document.querySelector("#contact-form");
const responseBox = document.querySelector("#response");
const statusBadge = document.querySelector("#status-badge");
const responseTime = document.querySelector("#response-time");
const schemaState = document.querySelector("#schema-state");

let fields = [];
let apiBasePath = "/api/v1";

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

const request = async (path) => {
  const baseURL = new URL(apiBaseInput.value);
  const basePath = baseURL.pathname.replace(/\/$/, "");
  const resolvedAPIBasePath = basePath && basePath !== "" ? "/api/v1" : apiBasePath;
  const res = await fetch(`${apiBaseInput.value}${resolvedAPIBasePath}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload()),
  });
  const body = await res.json().catch(() => ({}));
  setResponse(`${res.status} ${res.statusText}`, body);
};

const loadSchema = async () => {
  schemaState.textContent = "Loading form schema";
  const res = await fetch(`${apiBaseInput.value}/api/v1/schema`);
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

loadSchema().catch((error) => {
  schemaState.textContent = "Schema load failed";
  setResponse("Error", { error: error.message });
});
