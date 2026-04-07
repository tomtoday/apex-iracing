/**
 * APEX Frontend - Single Page App
 * Handles authentication, endpoint calls, and response display
 */

// Global state
let currentResponse = null;
let currentChunkIndex = 0;
let totalChunks = 1;

/**
 * Initialize the app on page load
 */
document.addEventListener("DOMContentLoaded", function () {
    checkAuthStatus();
});

/**
 * Check authentication status and update UI accordingly
 */
async function checkAuthStatus() {
    try {
        const response = await fetch("/api/auth-status");
        const data = await response.json();

        const loginBtn = document.getElementById("login-btn");
        const authStatus = document.getElementById("auth-status");
        const mainContent = document.getElementById("main-content");
        const notAuthMessage = document.getElementById("not-auth-message");

        if (data.authenticated) {
            // User is authenticated
            loginBtn.style.display = "none";
            authStatus.textContent = "✅ Authenticated";
            authStatus.className = "auth-status authenticated";
            mainContent.style.display = "block";
            notAuthMessage.style.display = "none";
        } else {
            // User is not authenticated
            loginBtn.style.display = "block";
            loginBtn.onclick = () => (window.location.href = "/login");
            authStatus.textContent = "❌ Not authenticated";
            authStatus.className = "auth-status not-authenticated";
            mainContent.style.display = "none";
            notAuthMessage.style.display = "block";
        }
    } catch (error) {
        console.error("Failed to check auth status:", error);
    }
}

/**
 * Fetch member info endpoint
 */
async function fetchMemberInfo() {
    const memberId = document.getElementById("member-id").value;
    let url = "/api/member-info";
    if (memberId) {
        url += `?member_id=${memberId}`;
    }

    await callEndpoint(url, "Member Info");
}

/**
 * Fetch cars endpoint
 */
async function fetchCars() {
    await callEndpoint("/api/cars", "Car List");
}

/**
 * Fetch search series endpoint
 */
async function fetchSearchSeries() {
    const seasonYear = document.getElementById("season-year").value;
    const seasonQuarter = document.getElementById("season-quarter").value;
    
    let url = "/api/search-series";
    const params = new URLSearchParams();
    
    if (seasonYear) params.append("season_year", seasonYear);
    if (seasonQuarter) params.append("season_quarter", seasonQuarter);
    
    if (params.toString()) {
        url += "?" + params.toString();
    }
    
    await callEndpoint(url, "Search Series");
}

/**
 * Generic endpoint call handler
 */
async function callEndpoint(url, name) {
    showLoading(true);
    hideError();

    try {
        const response = await fetch(url);

        if (response.status === 401) {
            showError("Not authenticated. Please log in.");
            checkAuthStatus();
            return;
        }

        const data = await response.json();

        if (data.error) {
            showError(data.error);
        } else {
            currentResponse = data;
            currentChunkIndex = data.current_chunk || 0;
            totalChunks = data.chunk_info?.total_chunks || 1;
            displayResponse(data);
        }
    } catch (error) {
        showError(`Failed to fetch: ${error.message}`);
    } finally {
        showLoading(false);
    }
}

/**
 * Display response data
 */
function displayResponse(data) {
    const responseSection = document.getElementById("response-section");
    const responseType = document.getElementById("response-type");
    const responseExpires = document.getElementById("response-expires");
    const responseExpiresP = document.getElementById("response-expires-p");
    const responseJson = document.getElementById("response-json");
    const chunkNav = document.getElementById("chunk-nav");
    const chunkInfo = document.getElementById("chunk-info");

    // Show response section
    responseSection.style.display = "block";

    // Display type
    if (data.type === "simple_s3") {
        responseType.textContent = "Simple S3 Link (Auto-fetched)";
        if (data.expires) {
            responseExpires.textContent = data.expires;
            responseExpiresP.style.display = "block";
        } else {
            responseExpiresP.style.display = "none";
        }
    } else if (data.type === "chunked") {
        responseType.textContent = "Chunked Data (Auto-fetched)";
        responseExpiresP.style.display = "none";
        chunkNav.style.display = "block";
        chunkInfo.textContent = `${currentChunkIndex + 1} of ${totalChunks}`;
        updateChunkButtons();
    } else if (data.type === "chunked_data") {
        responseType.textContent = "Chunk " + (currentChunkIndex + 1);
        chunkNav.style.display = "block";
        chunkInfo.textContent = `${currentChunkIndex + 1} of ${totalChunks}`;
        updateChunkButtons();
    } else if (data.type === "direct") {
        responseType.textContent = "Direct Response";
        responseExpiresP.style.display = "none";
        chunkNav.style.display = "none";
    }

    // Display JSON
    responseJson.textContent = JSON.stringify(data.data, null, 2);

    // Try to display as table if it's an array
    displayAsTable(data.data);
}

/**
 * Display data as table if possible
 */
function displayAsTable(data) {
    const tableBtn = document.getElementById("table-btn");
    const responseTable = document.getElementById("response-table");

    // Clear previous table
    responseTable.innerHTML = "";

    // Check if data is an array of objects
    if (Array.isArray(data) && data.length > 0 && typeof data[0] === "object") {
        tableBtn.style.display = "inline-block";

        const table = document.createElement("table");
        table.className = "data-table";

        // Create header
        const keys = Object.keys(data[0]);
        const thead = document.createElement("thead");
        const headerRow = document.createElement("tr");

        keys.forEach((key) => {
            const th = document.createElement("th");
            th.textContent = key;
            headerRow.appendChild(th);
        });
        thead.appendChild(headerRow);
        table.appendChild(thead);

        // Create body (limit to first 100 rows for performance)
        const tbody = document.createElement("tbody");
        const rowsToShow = Math.min(data.length, 100);

        for (let i = 0; i < rowsToShow; i++) {
            const row = document.createElement("tr");
            keys.forEach((key) => {
                const td = document.createElement("td");
                const value = data[i][key];
                td.textContent = value !== null && value !== undefined ? String(value).substring(0, 50) : "—";
                row.appendChild(td);
            });
            tbody.appendChild(row);
        }

        if (rowsToShow < data.length) {
            const row = document.createElement("tr");
            const td = document.createElement("td");
            td.colSpan = keys.length;
            td.textContent = `... and ${data.length - rowsToShow} more rows`;
            td.style.textAlign = "center";
            td.style.fontStyle = "italic";
            row.appendChild(td);
            tbody.appendChild(row);
        }

        table.appendChild(tbody);
        responseTable.appendChild(table);
    } else {
        tableBtn.style.display = "none";
    }
}

/**
 * Switch display tabs (JSON vs Table)
 */
function switchTab(tab) {
    const jsonDisplay = document.getElementById("response-json");
    const tableDisplay = document.getElementById("response-table");
    const buttons = document.querySelectorAll(".tab-btn");

    if (tab === "json") {
        jsonDisplay.classList.add("active");
        tableDisplay.classList.remove("active");
    } else {
        jsonDisplay.classList.remove("active");
        tableDisplay.classList.add("active");
    }

    buttons.forEach((btn) => {
        if (btn.textContent === (tab === "json" ? "JSON" : "Table")) {
            btn.classList.add("active");
        } else {
            btn.classList.remove("active");
        }
    });
}

/**
 * Navigate to previous chunk
 */
async function previousChunk() {
    if (currentChunkIndex > 0) {
        currentChunkIndex--;
        await fetchChunk();
    }
}

/**
 * Navigate to next chunk
 */
async function nextChunk() {
    if (currentChunkIndex < totalChunks - 1) {
        currentChunkIndex++;
        await fetchChunk();
    }
}

/**
 * Fetch a specific chunk
 */
async function fetchChunk() {
    if (!currentResponse || !currentResponse.chunk_info) {
        showError("No chunk metadata available");
        return;
    }

    const chunkFiles = JSON.stringify(currentResponse.chunk_info.chunk_files);
    const baseUrl = currentResponse.chunk_info.base_url || "";

    if (!baseUrl) {
        showError("Missing base URL for chunks");
        return;
    }

    showLoading(true);
    hideError();

    try {
        const params = new URLSearchParams({
            files: chunkFiles,
            base_url: baseUrl,
        });

        const response = await fetch(`/api/search-chunk/${currentChunkIndex}?${params}`);
        const data = await response.json();

        if (data.error) {
            showError(data.error);
        } else {
            // Update current response with new chunk data
            currentResponse.data = data.data;
            currentResponse.current_chunk = data.current_chunk;

            // Store original chunk_info if not already
            if (!currentResponse.chunk_info.base_url) {
                currentResponse.chunk_info.base_url = baseUrl;
            }

            displayResponse(currentResponse);
        }
    } catch (error) {
        showError(`Failed to fetch chunk: ${error.message}`);
    } finally {
        showLoading(false);
    }
}

/**
 * Update chunk navigation buttons
 */
function updateChunkButtons() {
    const prevBtn = document.getElementById("prev-chunk-btn");
    const nextBtn = document.getElementById("next-chunk-btn");

    prevBtn.disabled = currentChunkIndex === 0;
    nextBtn.disabled = currentChunkIndex >= totalChunks - 1;
}

/**
 * Show loading indicator
 */
function showLoading(show) {
    const loading = document.getElementById("loading");
    loading.style.display = show ? "flex" : "none";
}

/**
 * Show error message
 */
function showError(message) {
    const errorDisplay = document.getElementById("error-display");
    const errorMessage = document.getElementById("error-message");
    errorMessage.textContent = message;
    errorDisplay.style.display = "block";
}

/**
 * Hide error message
 */
function hideError() {
    document.getElementById("error-display").style.display = "none";
}
