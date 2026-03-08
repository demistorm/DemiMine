const API_BASE = "";
class ApiClient {
  baseUrl;
  token = null;
  constructor(baseUrl) {
    this.baseUrl = baseUrl;
    if (typeof window !== "undefined") {
      this.token = localStorage.getItem("token");
    }
  }
  setToken(token) {
    this.token = token;
    if (typeof window !== "undefined") {
      if (token) {
        localStorage.setItem("token", token);
      } else {
        localStorage.removeItem("token");
      }
    }
  }
  async request(endpoint, options = {}) {
    const url = `${this.baseUrl}${endpoint}`;
    const headers = {
      "Content-Type": "application/json",
      ...options.headers
    };
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    const response = await fetch(url, {
      ...options,
      headers
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({
        error: "unknown_error",
        message: response.statusText
      }));
      throw new Error(error.message || error.error);
    }
    if (response.status === 204) {
      return {};
    }
    return response.json();
  }
  async get(endpoint) {
    return this.request(endpoint);
  }
  async post(endpoint, data) {
    return this.request(endpoint, {
      method: "POST",
      body: data ? JSON.stringify(data) : void 0
    });
  }
  async patch(endpoint, data) {
    return this.request(endpoint, {
      method: "PATCH",
      body: JSON.stringify(data)
    });
  }
  async put(endpoint, data) {
    return this.request(endpoint, {
      method: "PUT",
      body: JSON.stringify(data)
    });
  }
  async delete(endpoint) {
    return this.request(endpoint, {
      method: "DELETE"
    });
  }
  async upload(endpoint, file) {
    const formData = new FormData();
    formData.append("file", file);
    const url = `${this.baseUrl}${endpoint}`;
    const headers = {};
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    const response = await fetch(url, {
      method: "POST",
      headers,
      body: formData
    });
    if (!response.ok) {
      throw new Error("Upload failed");
    }
    return response.json();
  }
}
const api = new ApiClient(API_BASE);
export {
  api as a
};
