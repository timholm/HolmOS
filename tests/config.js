/**
 * HolmOS Test Configuration
 *
 * Configuration file for all test suites
 */

const config = {
    // Cluster configuration
    cluster: {
        host: process.env.HOLMOS_CLUSTER_HOST || '192.168.8.197',
        ingressPort: process.env.HOLMOS_INGRESS_PORT || 80,
        nodePort: process.env.HOLMOS_NODEPORT || 30080,
        namespace: 'holm',
    },

    // Timeouts (in milliseconds)
    timeouts: {
        health: 5000,
        api: 10000,
        load: 30000,
        websocket: 15000,
    },

    // Test credentials
    auth: {
        username: process.env.TEST_USERNAME || 'admin',
        password: process.env.TEST_PASSWORD || 'admin123',
    },

    // Catppuccin Mocha theme colors for dashboards
    colors: {
        base: '#1e1e2e',
        mantle: '#181825',
        crust: '#11111b',
        text: '#cdd6f4',
        subtext0: '#a6adc8',
        subtext1: '#bac2de',
        overlay0: '#6c7086',
        overlay1: '#7f849c',
        overlay2: '#9399b2',
        surface0: '#313244',
        surface1: '#45475a',
        surface2: '#585b70',
        blue: '#89b4fa',
        lavender: '#b4befe',
        sapphire: '#74c7ec',
        sky: '#89dceb',
        teal: '#94e2d5',
        green: '#a6e3a1',
        yellow: '#f9e2af',
        peach: '#fab387',
        maroon: '#eba0ac',
        red: '#f38ba8',
        mauve: '#cba6f7',
        pink: '#f5c2e7',
        flamingo: '#f2cdcd',
        rosewater: '#f5e0dc',
    },

    // Service definitions with endpoints
    services: {
        // Core Infrastructure
        core: [
            { name: 'api-gateway', port: 8080, path: '/health' },
            { name: 'auth-gateway', port: 8080, path: '/health' },
            { name: 'gateway', port: 8080, path: '/health' },
        ],

        // Auth Services
        auth: [
            { name: 'auth-login', port: 80, path: '/health' },
            { name: 'auth-logout', port: 80, path: '/health' },
            { name: 'auth-refresh', port: 80, path: '/health' },
            { name: 'auth-register', port: 80, path: '/health' },
            { name: 'auth-token-validate', port: 8080, path: '/health' },
        ],

        // File Services
        files: [
            { name: 'file-list', port: 8080, path: '/health' },
            { name: 'file-upload', port: 8080, path: '/health' },
            { name: 'file-download', port: 8080, path: '/health' },
            { name: 'file-delete', port: 8080, path: '/health' },
            { name: 'file-copy', port: 8080, path: '/health' },
            { name: 'file-move', port: 8080, path: '/health' },
            { name: 'file-mkdir', port: 8080, path: '/health' },
            { name: 'file-meta', port: 8080, path: '/health' },
            { name: 'file-search', port: 8080, path: '/health' },
            { name: 'file-compress', port: 8080, path: '/health' },
            { name: 'file-decompress', port: 8080, path: '/health' },
            { name: 'file-preview', port: 8080, path: '/health' },
            { name: 'file-thumbnail', port: 8080, path: '/health' },
            { name: 'file-web', port: 8080, path: '/health' },
            { name: 'file-web-nautilus', port: 80, path: '/health' },
        ],

        // Terminal Services
        terminal: [
            { name: 'terminal', port: 8080, path: '/health' },
            { name: 'terminal-host-add', port: 8080, path: '/health' },
            { name: 'terminal-host-list', port: 8080, path: '/health' },
            { name: 'terminal-web', port: 8080, path: '/health' },
        ],

        // Cluster Management
        cluster: [
            { name: 'cluster-manager', port: 8080, path: '/health' },
            { name: 'cluster-apt-update', port: 8080, path: '/health' },
            { name: 'cluster-node-list', port: 8080, path: '/health' },
            { name: 'cluster-node-ping', port: 8080, path: '/health' },
            { name: 'cluster-reboot-exec', port: 8080, path: '/health' },
        ],

        // Registry Services
        registry: [
            { name: 'registry-ui', port: 8080, path: '/health' },
            { name: 'registry-list-repos', port: 8080, path: '/health' },
            { name: 'registry-list-tags', port: 8080, path: '/health' },
        ],

        // Settings Services
        settings: [
            { name: 'settings-web', port: 8080, path: '/health' },
            { name: 'settings-theme', port: 8080, path: '/health' },
            { name: 'settings-tabs', port: 8080, path: '/health' },
            { name: 'settings-backup', port: 8080, path: '/health' },
            { name: 'settings-restore', port: 8080, path: '/health' },
        ],

        // Audiobook Services
        audiobook: [
            { name: 'audiobook-web', port: 8080, path: '/health' },
            { name: 'audiobook-parse-epub', port: 8080, path: '/health' },
            { name: 'audiobook-chunk-text', port: 8080, path: '/health' },
            { name: 'audiobook-tts-convert', port: 8080, path: '/health' },
            { name: 'audiobook-audio-concat', port: 8080, path: '/health' },
            { name: 'audiobook-audio-normalize', port: 8080, path: '/health' },
            { name: 'audiobook-upload-epub', port: 8080, path: '/health' },
            { name: 'audiobook-upload-txt', port: 8080, path: '/health' },
        ],

        // Apps
        apps: [
            { name: 'app-store', port: 80, path: '/health' },
            { name: 'calculator-app', port: 80, path: '/health' },
            { name: 'clock-app', port: 80, path: '/health' },
            { name: 'contacts-app', port: 80, path: '/health' },
            { name: 'mail-app', port: 80, path: '/health' },
            { name: 'notes-app', port: 80, path: '/health' },
            { name: 'photos-app', port: 80, path: '/health' },
            { name: 'reminders-app', port: 80, path: '/health' },
        ],

        // Agent Services
        agents: [
            { name: 'agent-orchestrator', port: 80, path: '/health' },
            { name: 'agent-router', port: 80, path: '/health' },
            { name: 'config-agent', port: 5000, path: '/health' },
            { name: 'guardian-agent', port: 80, path: '/health' },
        ],

        // Platform Services
        platform: [
            { name: 'chat-hub', port: 8080, path: '/health' },
            { name: 'echo', port: 80, path: '/health' },
            { name: 'holmos-shell', port: 80, path: '/health' },
            { name: 'nova', port: 80, path: '/health' },
            { name: 'scribe', port: 80, path: '/health' },
            { name: 'vault', port: 80, path: '/health' },
        ],

        // DevOps Services
        devops: [
            { name: 'build-orchestrator', port: 80, path: '/health' },
            { name: 'cicd-controller', port: 5000, path: '/health' },
            { name: 'config-server', port: 8080, path: '/health' },
            { name: 'deploy-controller', port: 80, path: '/health' },
            { name: 'gitops-sync', port: 80, path: '/health' },
            { name: 'secret-manager', port: 8080, path: '/health' },
        ],

        // Monitoring Services
        monitoring: [
            { name: 'alerting', port: 80, path: '/health' },
            { name: 'log-aggregator', port: 80, path: '/health' },
            { name: 'metrics-collector', port: 8080, path: '/health' },
            { name: 'metrics-dashboard', port: 8080, path: '/health' },
            { name: 'test-dashboard', port: 8080, path: '/health' },
        ],

        // Backup Services
        backup: [
            { name: 'backup-scheduler', port: 8080, path: '/health' },
            { name: 'backup-storage', port: 80, path: '/health' },
            { name: 'backup-dashboard', port: 8080, path: '/health' },
            { name: 'restore-manager', port: 8080, path: '/health' },
        ],

        // Notification Services
        notification: [
            { name: 'notification-hub', port: 8080, path: '/health' },
            { name: 'notification-email', port: 8080, path: '/health' },
            { name: 'notification-queue', port: 80, path: '/health' },
            { name: 'notification-webhook', port: 8080, path: '/health' },
        ],

        // User Services
        user: [
            { name: 'user-activity', port: 80, path: '/health' },
            { name: 'user-preferences', port: 80, path: '/health' },
            { name: 'user-profile', port: 80, path: '/health' },
        ],

        // Infrastructure
        infra: [
            { name: 'cache-service', port: 8080, path: '/health' },
            { name: 'rate-limiter', port: 8080, path: '/health' },
        ],

        // Git Services
        git: [
            { name: 'gitea', port: 3000, path: '/' },
            { name: 'holm-git', port: 8080, path: '/health' },
        ],
    },

    // Load test configuration
    loadTest: {
        concurrentUsers: [10, 50, 100, 200],
        requestsPerUser: 100,
        rampUpTime: 5000,  // ms
        holdTime: 30000,   // ms
        rampDownTime: 5000, // ms
    },

    // Report configuration
    report: {
        outputDir: './reports',
        formats: ['json', 'html'],
        includeHistory: true,
        historyDays: 30,
    }
};

// Helper function to build service URL
config.getServiceUrl = (serviceName, port = 80, path = '/health') => {
    return `http://${serviceName}.${config.cluster.namespace}.svc.cluster.local:${port}${path}`;
};

// Helper function to build external URL
config.getExternalUrl = (path = '/') => {
    return `http://${config.cluster.host}:${config.cluster.ingressPort}${path}`;
};

// Helper to get all services as flat list
config.getAllServices = () => {
    const all = [];
    for (const category of Object.keys(config.services)) {
        for (const service of config.services[category]) {
            all.push({ ...service, category });
        }
    }
    return all;
};

module.exports = config;
