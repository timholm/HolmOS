// HolmOS Performance Test Configuration
// Optimized for Raspberry Pi cluster hardware constraints

export const CONFIG = {
  // Cluster endpoint
  baseUrl: 'http://192.168.8.197',

  // Service registry from services.yaml
  services: {
    // Core Entry Points
    'holmos-shell': { port: 30000, category: 'core', replicas: 2 },
    'claude-pod': { port: 30001, category: 'core', replicas: 1 },
    'app-store': { port: 30002, category: 'core', replicas: 1 },
    'chat-hub': { port: 30003, category: 'core', replicas: 1 },

    // AI Agents
    'nova': { port: 30004, category: 'agent', replicas: 1 },
    'merchant': { port: 30005, category: 'agent', replicas: 1 },
    'pulse': { port: 30006, category: 'agent', replicas: 1 },
    'gateway': { port: 30008, category: 'agent', replicas: 2 },
    'scribe': { port: 30860, category: 'agent', replicas: 1 },
    'vault': { port: 30870, category: 'agent', replicas: 1 },

    // Apps
    'clock-app': { port: 30007, category: 'app', replicas: 1 },
    'calculator-app': { port: 30010, category: 'app', replicas: 1 },
    'file-web-nautilus': { port: 30088, category: 'app', replicas: 1 },
    'settings-web': { port: 30600, category: 'app', replicas: 1 },
    'audiobook-web': { port: 30700, category: 'app', replicas: 1 },
    'terminal-web': { port: 30800, category: 'app', replicas: 1 },

    // Infrastructure
    'holm-git': { port: 30009, category: 'devops', replicas: 1 },
    'cicd-controller': { port: 30020, category: 'devops', replicas: 1 },
    'deploy-controller': { port: 30021, category: 'devops', replicas: 1 },
    'registry-ui': { port: 31750, category: 'devops', replicas: 1 },

    // Admin/Monitoring
    'cluster-manager': { port: 30502, category: 'admin', replicas: 1 },
    'backup-dashboard': { port: 30850, category: 'admin', replicas: 1 },
    'test-dashboard': { port: 30900, category: 'monitoring', replicas: 1 },
    'metrics-dashboard': { port: 30950, category: 'monitoring', replicas: 1 },
  },

  // Performance thresholds for Raspberry Pi hardware
  // Conservative targets given ARM CPU and limited memory
  thresholds: {
    // Response time thresholds (milliseconds)
    http_req_duration: {
      p95: 2000,      // 95th percentile under 2s
      p99: 5000,      // 99th percentile under 5s
      avg: 500,       // Average under 500ms
    },

    // Health check specific (should be fast)
    health_check_duration: {
      p95: 500,       // Health checks under 500ms
      p99: 1000,
      avg: 100,
    },

    // Error rate thresholds
    http_req_failed: {
      rate: 0.01,     // Less than 1% error rate
    },

    // Throughput minimums (requests per second)
    throughput: {
      min: 10,        // At least 10 RPS per service
    },
  },

  // Load stages optimized for Pi hardware
  // Start low, ramp conservatively
  loadStages: {
    // Smoke test - basic functionality
    smoke: [
      { duration: '30s', target: 1 },
    ],

    // Light load - normal operation
    light: [
      { duration: '30s', target: 5 },
      { duration: '1m', target: 5 },
      { duration: '30s', target: 0 },
    ],

    // Standard load - expected traffic
    standard: [
      { duration: '30s', target: 10 },
      { duration: '2m', target: 10 },
      { duration: '30s', target: 20 },
      { duration: '1m', target: 20 },
      { duration: '30s', target: 0 },
    ],

    // Stress test - find breaking point
    stress: [
      { duration: '30s', target: 10 },
      { duration: '1m', target: 20 },
      { duration: '1m', target: 30 },
      { duration: '1m', target: 40 },
      { duration: '2m', target: 50 },
      { duration: '1m', target: 0 },
    ],

    // Spike test - sudden traffic burst
    spike: [
      { duration: '30s', target: 5 },
      { duration: '10s', target: 50 },
      { duration: '1m', target: 50 },
      { duration: '10s', target: 5 },
      { duration: '30s', target: 0 },
    ],

    // Soak test - extended duration
    soak: [
      { duration: '1m', target: 10 },
      { duration: '10m', target: 10 },
      { duration: '1m', target: 0 },
    ],
  },
};

// Helper to build service URL
export function serviceUrl(serviceName, path = '') {
  const service = CONFIG.services[serviceName];
  if (!service) {
    throw new Error(`Unknown service: ${serviceName}`);
  }
  return `${CONFIG.baseUrl}:${service.port}${path}`;
}

// Helper to get services by category
export function getServicesByCategory(category) {
  return Object.entries(CONFIG.services)
    .filter(([_, config]) => config.category === category)
    .map(([name, config]) => ({ name, ...config }));
}

// Helper to get all service names
export function getAllServices() {
  return Object.keys(CONFIG.services);
}

export default CONFIG;
