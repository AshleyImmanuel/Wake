#!/usr/bin/env node
const { spawnSync } = require('child_process');
const path = require('path');
const os = require('os');

const ext = os.platform() === 'win32' ? '.exe' : '';
const binPath = path.join(__dirname, 'wake' + ext);

const result = spawnSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
    console.error("Failed to start Wake: ", result.error.message);
    process.exit(1);
}
process.exit(result.status || 0);
