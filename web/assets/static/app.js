/**
 * APEX - iRacing API Explorer
 * Dynamic endpoint explorer driven by /api/docs
 */

// ── State ──────────────────────────────────────────────────────────────────
let docs = null;
let selectedEndpoint = null;   // { path, data }
let currentResponse = null;
let currentChunkIndex = 0;
let totalChunks = 1;

// ── Boot ───────────────────────────────────────────────────────────────────
document.addEventListener("DOMContentLoaded", () => init());

async function init() {
    const res = await fetch("/api/auth-status");
    const { authenticated } = await res.json();

    const loginBtn = document.getElementById("login-btn");
    const logoutBtn = document.getElementById("logout-btn");
    const authStatus = document.getElementById("auth-status");

    if (authenticated) {
        authStatus.textContent = "Authenticated";
        authStatus.className = "auth-status authenticated";
        logoutBtn.style.display = "inline-block";
        logoutBtn.onclick = () => (window.location.href = "/logout");
        await loadDocs();
    } else {
        authStatus.textContent = "Not authenticated";
        authStatus.className = "auth-status not-authenticated";
        loginBtn.style.display = "inline-block";
        loginBtn.onclick = () => (window.location.href = "/login");
        document.getElementById("welcome-screen").style.display = "none";
        document.getElementById("not-auth-screen").style.display = "flex";
    }
}

// ── Docs / sidebar ─────────────────────────────────────────────────────────
async function loadDocs() {
    const res = await fetch("/api/docs");
    docs = await res.json();
    renderSidebar();

    // wire up search
    document.getElementById("endpoint-search").addEventListener("input", (e) => {
        renderSidebar(e.target.value.trim());
    });
}

function renderSidebar(filter = "") {
    const nav = document.getElementById("endpoint-nav");
    nav.innerHTML = "";
    const fl = filter.toLowerCase();

    for (const [category, endpoints] of Object.entries(docs)) {
        const names = Object.keys(endpoints).filter((name) => {
            if (!fl) return true;
            return category.includes(fl) || name.includes(fl) || `${category}/${name}`.includes(fl);
        });
        if (!names.length) continue;

        const section = document.createElement("div");
        section.className = "nav-section";

        const heading = document.createElement("div");
        heading.className = "nav-category";
        heading.textContent = category.replace(/_/g, " ");
        section.appendChild(heading);

        for (const name of names) {
            const item = document.createElement("div");
            item.className = "nav-item";
            if (selectedEndpoint && selectedEndpoint.path === `${category}/${name}`) {
                item.classList.add("active");
            }
            item.textContent = name.replace(/_/g, " ");
            item.dataset.endpoint = `${category}/${name}`;
            item.addEventListener("click", () => selectEndpoint(`${category}/${name}`));
            section.appendChild(item);
        }
        nav.appendChild(section);
    }
}

// ── Endpoint selection ─────────────────────────────────────────────────────
function selectEndpoint(path) {
    const [category, name] = path.split("/");
    const data = docs[category][name];

    selectedEndpoint = { path, data };
    currentResponse = null;
    currentChunkIndex = 0;
    totalChunks = 1;

    // update sidebar active state
    document.querySelectorAll(".nav-item").forEach((el) => {
        el.classList.toggle("active", el.dataset.endpoint === path);
    });

    // populate header
    document.getElementById("endpoint-title").textContent = path.replace("/", " / ");
    document.getElementById("endpoint-url").textContent = data.link;

    const noteEl = document.getElementById("endpoint-note");
    if (data.note) {
        noteEl.textContent = data.note;
        noteEl.style.display = "block";
    } else {
        noteEl.style.display = "none";
    }

    renderParamsForm(data.parameters || {});

    // reset response area
    document.getElementById("response-section").style.display = "none";
    document.getElementById("error-display").style.display = "none";

    // show panel
    document.getElementById("welcome-screen").style.display = "none";
    document.getElementById("endpoint-panel").style.display = "block";
}

// ── Params form ────────────────────────────────────────────────────────────
function renderParamsForm(parameters) {
    const form = document.getElementById("params-form");
    form.innerHTML = "";

    const entries = Object.entries(parameters);
    if (!entries.length) {
        form.innerHTML = '<p class="no-params">No parameters — just click Call Endpoint.</p>';
        return;
    }

    for (const [name, info] of entries) {
        const group = document.createElement("div");
        group.className = "form-group";

        const label = document.createElement("label");
        label.htmlFor = `param-${name}`;
        label.innerHTML = name.replace(/_/g, " ");
        if (info.required) {
            label.innerHTML += ' <span class="required">*</span>';
        }

        let input;
        if (info.type === "boolean") {
            input = document.createElement("select");
            input.className = "form-input";
            input.innerHTML = '<option value="">—</option><option value="true">true</option><option value="false">false</option>';
        } else {
            input = document.createElement("input");
            input.className = "form-input";
            input.type = info.type === "number" ? "number" : "text";
            if (info.type === "numbers") {
                input.placeholder = "comma-separated, e.g. 1,2,3";
            }
        }
        input.id = `param-${name}`;
        input.dataset.paramName = name;

        group.appendChild(label);
        group.appendChild(input);

        if (info.note) {
            const hint = document.createElement("small");
            hint.className = "param-hint";
            hint.textContent = info.note;
            group.appendChild(hint);
        }

        form.appendChild(group);
    }
}

// ── API call ───────────────────────────────────────────────────────────────
async function callSelectedEndpoint() {
    if (!selectedEndpoint) return;

    const params = new URLSearchParams({ _endpoint: selectedEndpoint.path });
    document.querySelectorAll("[data-param-name]").forEach((input) => {
        const val = input.value.trim();
        if (val) params.append(input.dataset.paramName, val);
    });

    showLoading(true);
    hideError();
    document.getElementById("response-section").style.display = "none";

    try {
        const res = await fetch(`/api/proxy?${params}`);
        if (res.status === 401) {
            window.location.href = "/login";
            return;
        }
        const data = await res.json();
        if (data.error) {
            showError(data.error);
        } else {
            currentResponse = data;
            currentChunkIndex = data.current_chunk || 0;
            totalChunks = data.chunk_info?.total_chunks || 1;
            displayResponse(data);
        }
    } catch (err) {
        showError(`Request failed: ${err.message}`);
    } finally {
        showLoading(false);
    }
}

// ── Response display ───────────────────────────────────────────────────────
function displayResponse(data) {
    document.getElementById("response-section").style.display = "block";

    // type badge
    const typeEl = document.getElementById("response-type");
    const chunkCounter = document.getElementById("chunk-counter");
    const chunkNav = document.getElementById("chunk-nav");

    if (data.type === "chunked" || data.type === "chunked_data") {
        typeEl.textContent = "chunked";
        chunkCounter.textContent = `chunk ${currentChunkIndex + 1} of ${totalChunks}`;
        chunkCounter.style.display = "inline";
        chunkNav.style.display = "flex";
        updateChunkButtons();
    } else {
        typeEl.textContent = data.type || "direct";
        chunkCounter.style.display = "none";
        chunkNav.style.display = "none";
    }

    // JSON
    document.getElementById("response-json").textContent = JSON.stringify(data.data, null, 2);

    // Table (if array)
    renderTable(data.data);

    // default to JSON tab
    switchTab("json");
}

function renderTable(data) {
    const tableBtn = document.getElementById("table-btn");
    const tableEl = document.getElementById("response-table");
    tableEl.innerHTML = "";

    if (!Array.isArray(data) || !data.length || typeof data[0] !== "object") {
        tableBtn.style.display = "none";
        return;
    }

    tableBtn.style.display = "inline-block";

    const table = document.createElement("table");
    table.className = "data-table";

    const keys = Object.keys(data[0]);
    const thead = table.createTHead();
    const headerRow = thead.insertRow();
    keys.forEach((k) => {
        const th = document.createElement("th");
        th.textContent = k;
        headerRow.appendChild(th);
    });

    const tbody = table.createTBody();
    const limit = Math.min(data.length, 100);
    for (let i = 0; i < limit; i++) {
        const row = tbody.insertRow();
        keys.forEach((k) => {
            const td = row.insertCell();
            const v = data[i][k];
            td.textContent = v !== null && v !== undefined ? String(v).substring(0, 60) : "—";
        });
    }

    if (data.length > limit) {
        const row = tbody.insertRow();
        const td = row.insertCell();
        td.colSpan = keys.length;
        td.className = "table-overflow";
        td.textContent = `… ${data.length - limit} more rows`;
    }

    tableEl.appendChild(table);
}

// ── Tabs ───────────────────────────────────────────────────────────────────
function switchTab(tab) {
    const jsonEl = document.getElementById("response-json");
    const tableEl = document.getElementById("response-table");
    document.querySelectorAll(".tab-btn").forEach((btn) => btn.classList.remove("active"));

    if (tab === "json") {
        jsonEl.classList.add("active");
        tableEl.classList.remove("active");
        document.querySelectorAll(".tab-btn")[0].classList.add("active");
    } else {
        tableEl.classList.add("active");
        jsonEl.classList.remove("active");
        document.querySelectorAll(".tab-btn")[1].classList.add("active");
    }
}

// ── Chunk navigation ───────────────────────────────────────────────────────
async function previousChunk() {
    if (currentChunkIndex > 0) { currentChunkIndex--; await fetchChunk(); }
}

async function nextChunk() {
    if (currentChunkIndex < totalChunks - 1) { currentChunkIndex++; await fetchChunk(); }
}

async function fetchChunk() {
    if (!currentResponse?.chunk_info) { showError("No chunk metadata available"); return; }

    const { chunk_files, base_url } = currentResponse.chunk_info;
    if (!base_url) { showError("Missing base URL for chunks"); return; }

    showLoading(true);
    hideError();

    try {
        const params = new URLSearchParams({
            files: JSON.stringify(chunk_files),
            base_url,
        });
        const res = await fetch(`/api/search-chunk/${currentChunkIndex}?${params}`);
        const data = await res.json();
        if (data.error) {
            showError(data.error);
        } else {
            currentResponse.data = data.data;
            currentResponse.current_chunk = data.current_chunk;
            displayResponse(currentResponse);
        }
    } catch (err) {
        showError(`Failed to fetch chunk: ${err.message}`);
    } finally {
        showLoading(false);
    }
}

function updateChunkButtons() {
    document.getElementById("prev-chunk-btn").disabled = currentChunkIndex === 0;
    document.getElementById("next-chunk-btn").disabled = currentChunkIndex >= totalChunks - 1;
}

// ── Helpers ────────────────────────────────────────────────────────────────
function showLoading(show) {
    document.getElementById("loading").style.display = show ? "flex" : "none";
}

function showError(msg) {
    const el = document.getElementById("error-display");
    document.getElementById("error-message").textContent = msg;
    el.style.display = "block";
}

function hideError() {
    document.getElementById("error-display").style.display = "none";
}
