const os = require('os');
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const https = require('https');

const version = "1.2.0"; // Should match package.json version

let osName = os.platform();
let arch = os.arch();

if (osName === 'win32') osName = 'Windows';
else if (osName === 'darwin') osName = 'Darwin';
else if (osName === 'linux') osName = 'Linux';
else {
    console.error("Unsupported OS:", osName);
    process.exit(1);
}

if (arch === 'x64') arch = 'x86_64';
else if (arch === 'arm64') arch = 'arm64';
else if (arch === 'ia32') arch = 'i386';
else {
    console.error("Unsupported architecture:", arch);
    process.exit(1);
}

const ext = osName === 'Windows' ? 'zip' : 'tar.gz';
const filename = `Wake_${osName}_${arch}.${ext}`;
const url = `https://github.com/AshleyImmanuel/Wake/releases/download/v${version}/${filename}`;

console.log(`Downloading Wake v${version} for ${osName} ${arch}...`);
console.log(`Fetching from: ${url}`);

const dest = path.join(__dirname, filename);

function download(url, dest) {
    return new Promise((resolve, reject) => {
        https.get(url, (res) => {
            if (res.statusCode === 301 || res.statusCode === 302) {
                return resolve(download(res.headers.location, dest));
            }
            if (res.statusCode !== 200) {
                return reject(new Error(`Failed to download: ${res.statusCode}`));
            }
            const file = fs.createWriteStream(dest);
            res.pipe(file);
            file.on('finish', () => {
                file.close(resolve);
            });
        }).on('error', (err) => {
            fs.unlink(dest, () => reject(err));
        });
    });
}

async function install() {
    try {
        await download(url, dest);
        console.log("Download complete. Extracting...");
        
        const installDir = __dirname;
        const binName = osName === 'Windows' ? 'wake.exe' : 'wake';
        
        if (osName === 'Windows') {
            execSync(`tar -xf "${dest}"`, { cwd: installDir, stdio: 'inherit' });
        } else {
            execSync(`tar -xzf "${dest}"`, { cwd: installDir, stdio: 'inherit' });
        }
        
        // GoReleaser may place the binary at the root of the archive.
        // Ensure it's in installDir.
        const binPath = path.join(installDir, binName);
        if (!fs.existsSync(binPath)) {
            // Search one level deep for the binary
            const entries = fs.readdirSync(installDir);
            for (const entry of entries) {
                const candidate = path.join(installDir, entry, binName);
                if (fs.existsSync(candidate)) {
                    fs.renameSync(candidate, binPath);
                    break;
                }
            }
        }
        
        if (!osName.startsWith('Windows')) {
            execSync(`chmod +x "${binPath}"`, { stdio: 'inherit' });
        }
        
        // Clean up archive
        fs.unlinkSync(dest);
        
        console.log('Wake installed successfully!');
    } catch(e) {
        console.error("Installation failed:", e.message);
        process.exit(1);
    }
}

install();
