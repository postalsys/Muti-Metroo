// API Client for Muti Metroo Dashboard

const API = {
    baseUrl: window.location.origin,

    async getDashboard() {
        const response = await fetch(`${this.baseUrl}/api/dashboard`);
        if (!response.ok) {
            throw new Error(`Dashboard API error: ${response.status}`);
        }
        return response.json();
    },

    async getTopology() {
        const response = await fetch(`${this.baseUrl}/api/topology`);
        if (!response.ok) {
            throw new Error(`Topology API error: ${response.status}`);
        }
        return response.json();
    },

    async getHealth() {
        const response = await fetch(`${this.baseUrl}/healthz`);
        if (!response.ok) {
            throw new Error(`Health API error: ${response.status}`);
        }
        return response.json();
    },

    async triggerRouteAdvertise() {
        const response = await fetch(`${this.baseUrl}/routes/advertise`, {
            method: 'POST'
        });
        if (!response.ok) {
            throw new Error(`Route advertise error: ${response.status}`);
        }
        return response.json();
    },

    async getMeshTest(forceRefresh = false) {
        const method = forceRefresh ? 'POST' : 'GET';
        const response = await fetch(`${this.baseUrl}/api/mesh-test`, { method });
        if (!response.ok) {
            throw new Error(`Mesh test error: ${response.status}`);
        }
        return response.json();
    }
};
