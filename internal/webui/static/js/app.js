// Main Application for Muti Metroo Dashboard

class Dashboard {
    constructor() {
        this.metroMap = null;
        this.pollInterval = 5000; // 5 seconds
        this.pollTimer = null;
    }

    async init() {
        // Initialize metro map
        const svgElement = document.getElementById('metro-svg');
        this.metroMap = new MetroMap(svgElement);

        // Initial data load
        await this.refresh();

        // Start polling
        this.startPolling();
    }

    async refresh() {
        try {
            // Fetch both dashboard and topology data
            const [dashboard, topology] = await Promise.all([
                API.getDashboard(),
                API.getTopology()
            ]);

            this.updateAgentInfo(dashboard.agent);
            this.updateStats(dashboard.stats, dashboard.routes);
            this.updateRoutesTable(dashboard.routes);
            this.metroMap.setRoutes(dashboard.routes);
            this.metroMap.update(topology);
            this.updateLastRefresh();
        } catch (error) {
            console.error('Failed to refresh dashboard:', error);
            this.showError(error.message);
        }
    }

    updateAgentInfo(agent) {
        const nameEl = document.getElementById('agent-name');
        const idEl = document.getElementById('agent-id');

        nameEl.textContent = agent.display_name || agent.short_id;
        idEl.textContent = agent.short_id;
    }

    updateStats(stats, routes) {
        document.getElementById('peer-count').textContent = stats.peer_count;
        document.getElementById('stream-count').textContent = stats.stream_count;

        // Count unique exit nodes (unique route origins)
        const exitNodes = new Set(routes?.map(r => r.origin_id) || []);
        document.getElementById('exit-count').textContent = exitNodes.size;
    }

    updateRoutesTable(routes) {
        const tbody = document.getElementById('routes-tbody');

        if (!routes || routes.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4" class="no-data">No routes</td></tr>';
            return;
        }

        tbody.innerHTML = routes.map(route => {
            // Build path display as arrow-separated string
            const pathStr = route.path_display ? route.path_display.join(' -> ') : route.origin;
            return `
                <tr>
                    <td>${route.network}</td>
                    <td title="${route.origin_id}">${route.origin}</td>
                    <td>${pathStr}</td>
                    <td>${route.hop_count}</td>
                </tr>
            `;
        }).join('');
    }

    updateLastRefresh() {
        const el = document.getElementById('last-update');
        const now = new Date();
        el.textContent = `Last update: ${now.toLocaleTimeString()}`;
    }

    showError(message) {
        console.error('Dashboard error:', message);
        // Could show a toast notification here
    }

    startPolling() {
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
        }
        this.pollTimer = setInterval(() => this.refresh(), this.pollInterval);
    }

    stopPolling() {
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = null;
        }
    }
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    const dashboard = new Dashboard();
    dashboard.init();

    // Make dashboard accessible for debugging
    window.dashboard = dashboard;
});
