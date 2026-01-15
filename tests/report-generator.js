#!/usr/bin/env node
/**
 * HolmOS Test Report Generator
 *
 * Generates HTML reports with Catppuccin theme
 */

const fs = require('fs');
const path = require('path');
const config = require('./config');

class ReportGenerator {
    constructor() {
        this.colors = config.colors;
    }

    generateHTML(report) {
        const timestamp = new Date(report.timestamp).toLocaleString();
        const duration = (report.duration / 1000).toFixed(2);
        const passRate = report.summary.passRate;
        const passRateColor = passRate >= 90 ? this.colors.green :
                             passRate >= 70 ? this.colors.yellow : this.colors.red;

        return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>HolmOS Test Report - ${timestamp}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: ${this.colors.base};
            color: ${this.colors.text};
            min-height: 100vh;
            line-height: 1.6;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 2rem;
        }

        header {
            text-align: center;
            margin-bottom: 2rem;
            padding: 2rem;
            background: linear-gradient(135deg, ${this.colors.mantle} 0%, ${this.colors.surface0} 100%);
            border-radius: 16px;
            border: 1px solid ${this.colors.surface1};
        }

        .logo {
            font-size: 3rem;
            margin-bottom: 0.5rem;
        }

        h1 {
            font-size: 2rem;
            color: ${this.colors.mauve};
            margin-bottom: 0.25rem;
        }

        .tagline {
            color: ${this.colors.subtext0};
            font-size: 1rem;
        }

        .meta-info {
            display: flex;
            justify-content: center;
            gap: 2rem;
            margin-top: 1rem;
            flex-wrap: wrap;
        }

        .meta-item {
            color: ${this.colors.subtext0};
            font-size: 0.9rem;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }

        .stat-card {
            background: ${this.colors.mantle};
            border: 1px solid ${this.colors.surface1};
            border-radius: 12px;
            padding: 1.5rem;
            text-align: center;
        }

        .stat-value {
            font-size: 2.5rem;
            font-weight: bold;
        }

        .stat-value.passed { color: ${this.colors.green}; }
        .stat-value.failed { color: ${this.colors.red}; }
        .stat-value.errors { color: ${this.colors.mauve}; }
        .stat-value.total { color: ${this.colors.blue}; }
        .stat-value.rate { color: ${passRateColor}; }

        .stat-label {
            color: ${this.colors.subtext0};
            font-size: 0.85rem;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-top: 0.5rem;
        }

        .section {
            background: ${this.colors.mantle};
            border: 1px solid ${this.colors.surface1};
            border-radius: 16px;
            padding: 1.5rem;
            margin-bottom: 1.5rem;
        }

        .section-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 1rem;
            padding-bottom: 1rem;
            border-bottom: 1px solid ${this.colors.surface0};
        }

        .section-title {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            color: ${this.colors.text};
            font-size: 1.25rem;
            font-weight: 600;
        }

        .suite-result {
            background: ${this.colors.surface0};
            border-radius: 12px;
            padding: 1rem;
            margin-bottom: 1rem;
        }

        .suite-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            cursor: pointer;
        }

        .suite-name {
            font-weight: 600;
            color: ${this.colors.text};
        }

        .suite-stats {
            display: flex;
            gap: 1rem;
            align-items: center;
        }

        .badge {
            font-size: 0.75rem;
            padding: 0.25rem 0.75rem;
            border-radius: 4px;
            font-weight: 600;
        }

        .badge-pass {
            background: ${this.colors.green}20;
            color: ${this.colors.green};
        }

        .badge-fail {
            background: ${this.colors.red}20;
            color: ${this.colors.red};
        }

        .badge-error {
            background: ${this.colors.mauve}20;
            color: ${this.colors.mauve};
        }

        .badge-skip {
            background: ${this.colors.yellow}20;
            color: ${this.colors.yellow};
        }

        .test-list {
            margin-top: 1rem;
            display: none;
        }

        .test-list.expanded {
            display: block;
        }

        .test-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 0.75rem;
            border-bottom: 1px solid ${this.colors.surface1};
        }

        .test-item:last-child {
            border-bottom: none;
        }

        .test-name {
            color: ${this.colors.text};
        }

        .test-time {
            color: ${this.colors.subtext0};
            font-size: 0.85rem;
        }

        .status-icon {
            width: 20px;
            height: 20px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 12px;
        }

        .status-icon.pass { background: ${this.colors.green}; color: ${this.colors.base}; }
        .status-icon.fail { background: ${this.colors.red}; color: ${this.colors.base}; }
        .status-icon.error { background: ${this.colors.mauve}; color: ${this.colors.base}; }
        .status-icon.skip { background: ${this.colors.yellow}; color: ${this.colors.base}; }

        .progress-bar {
            height: 8px;
            background: ${this.colors.surface1};
            border-radius: 4px;
            overflow: hidden;
            margin-top: 0.5rem;
        }

        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, ${this.colors.green} 0%, ${this.colors.teal} 100%);
            border-radius: 4px;
            transition: width 0.3s ease;
        }

        footer {
            text-align: center;
            padding: 2rem;
            color: ${this.colors.subtext0};
            font-size: 0.85rem;
        }

        footer a {
            color: ${this.colors.mauve};
            text-decoration: none;
        }

        .expand-btn {
            background: none;
            border: none;
            color: ${this.colors.subtext0};
            cursor: pointer;
            font-size: 1.2rem;
            transition: transform 0.2s;
        }

        .expand-btn.expanded {
            transform: rotate(90deg);
        }

        .chart-container {
            margin-top: 1rem;
            padding: 1rem;
            background: ${this.colors.surface0};
            border-radius: 8px;
        }

        .bar-chart {
            display: flex;
            align-items: flex-end;
            gap: 0.5rem;
            height: 150px;
        }

        .bar {
            flex: 1;
            background: ${this.colors.blue};
            border-radius: 4px 4px 0 0;
            min-height: 4px;
            position: relative;
        }

        .bar:hover {
            opacity: 0.8;
        }

        .bar-label {
            position: absolute;
            bottom: -25px;
            left: 50%;
            transform: translateX(-50%);
            font-size: 0.7rem;
            color: ${this.colors.subtext0};
            white-space: nowrap;
        }

        @media (max-width: 768px) {
            .container { padding: 1rem; }
            .stats-grid { grid-template-columns: repeat(2, 1fr); }
            .stat-value { font-size: 1.8rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">&#128202;</div>
            <h1>HolmOS Test Report</h1>
            <p class="tagline">End-to-End and Integration Test Results</p>
            <div class="meta-info">
                <span class="meta-item">Cluster: ${report.cluster.host}</span>
                <span class="meta-item">Namespace: ${report.cluster.namespace}</span>
                <span class="meta-item">Generated: ${timestamp}</span>
                <span class="meta-item">Duration: ${duration}s</span>
            </div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-value total">${report.summary.totalTests}</div>
                <div class="stat-label">Total Tests</div>
            </div>
            <div class="stat-card">
                <div class="stat-value passed">${report.summary.passed}</div>
                <div class="stat-label">Passed</div>
            </div>
            <div class="stat-card">
                <div class="stat-value failed">${report.summary.failed}</div>
                <div class="stat-label">Failed</div>
            </div>
            <div class="stat-card">
                <div class="stat-value errors">${report.summary.errors}</div>
                <div class="stat-label">Errors</div>
            </div>
            <div class="stat-card">
                <div class="stat-value rate">${passRate}%</div>
                <div class="stat-label">Pass Rate</div>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: ${passRate}%"></div>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <div class="section-title">Test Suite Results</div>
            </div>
            ${this.generateSuiteResults(report.results)}
        </div>

        <div class="section">
            <div class="section-header">
                <div class="section-title">Suite Summary Chart</div>
            </div>
            <div class="chart-container">
                <div class="bar-chart">
                    ${this.generateBarChart(report.results)}
                </div>
            </div>
        </div>

        <footer>
            <p>HolmOS Test Framework</p>
            <p>Report generated with Catppuccin Mocha theme</p>
        </footer>
    </div>

    <script>
        document.querySelectorAll('.suite-header').forEach(header => {
            header.addEventListener('click', () => {
                const suite = header.parentElement;
                const testList = suite.querySelector('.test-list');
                const btn = header.querySelector('.expand-btn');
                if (testList) {
                    testList.classList.toggle('expanded');
                    btn.classList.toggle('expanded');
                }
            });
        });
    </script>
</body>
</html>`;
    }

    generateSuiteResults(results) {
        return results.map(suite => {
            const summary = suite.summary || {};
            const hasFailed = (summary.failed || 0) + (summary.errors || 0) > 0;
            const statusClass = hasFailed ? 'fail' : 'pass';

            const tests = suite.tests || [];
            const testItems = tests.map(test => {
                const statusIcon = test.status === 'pass' ? '&#10003;' :
                                  test.status === 'fail' ? '&#10007;' :
                                  test.status === 'error' ? '!' : '-';
                return `
                    <div class="test-item">
                        <div style="display: flex; align-items: center; gap: 0.75rem;">
                            <span class="status-icon ${test.status}">${statusIcon}</span>
                            <span class="test-name">${test.name}</span>
                        </div>
                        <span class="test-time">${test.responseTime ? test.responseTime + 'ms' : ''}</span>
                    </div>
                `;
            }).join('');

            return `
                <div class="suite-result">
                    <div class="suite-header">
                        <span class="suite-name">${suite.suite}</span>
                        <div class="suite-stats">
                            <span class="badge badge-pass">${summary.passed || 0} passed</span>
                            ${(summary.failed || 0) > 0 ? `<span class="badge badge-fail">${summary.failed} failed</span>` : ''}
                            ${(summary.errors || 0) > 0 ? `<span class="badge badge-error">${summary.errors} errors</span>` : ''}
                            <button class="expand-btn">&#9654;</button>
                        </div>
                    </div>
                    ${tests.length > 0 ? `<div class="test-list">${testItems}</div>` : ''}
                </div>
            `;
        }).join('');
    }

    generateBarChart(results) {
        const maxPassed = Math.max(...results.map(r => r.summary?.passed || 0), 1);

        return results.map(suite => {
            const passed = suite.summary?.passed || 0;
            const height = (passed / maxPassed * 100).toFixed(0);
            const shortName = suite.suite.split(' ')[0].substring(0, 8);

            return `
                <div class="bar" style="height: ${height}%;" title="${suite.suite}: ${passed} passed">
                    <span class="bar-label">${shortName}</span>
                </div>
            `;
        }).join('');
    }

    async generate(reportPath, outputPath) {
        // Load JSON report
        const reportContent = fs.readFileSync(reportPath, 'utf8');
        const report = JSON.parse(reportContent);

        // Generate HTML
        const html = this.generateHTML(report);

        // Ensure output directory exists
        const outputDir = path.dirname(outputPath);
        if (!fs.existsSync(outputDir)) {
            fs.mkdirSync(outputDir, { recursive: true });
        }

        // Write HTML file
        fs.writeFileSync(outputPath, html);
        console.log(`HTML report generated: ${outputPath}`);

        return outputPath;
    }
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const inputFile = args.find(a => !a.startsWith('--')) ||
                      path.join(__dirname, 'reports', 'latest.json');
    const outputFile = args.find(a => a.startsWith('--output='))?.split('=')[1] ||
                       inputFile.replace('.json', '.html');

    if (!fs.existsSync(inputFile)) {
        console.error(`Input file not found: ${inputFile}`);
        console.error('Run tests with --save-report first, or specify an input file.');
        process.exit(1);
    }

    const generator = new ReportGenerator();
    await generator.generate(inputFile, outputFile);
}

if (require.main === module) {
    main().catch(err => {
        console.error('Report generation failed:', err);
        process.exit(1);
    });
}

module.exports = { ReportGenerator };
