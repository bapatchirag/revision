import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';

export function activate(context: vscode.ExtensionContext): void {
	context.subscriptions.push(
		vscode.commands.registerCommand('revision.open', () => openRevision(context))
	);
}

export function deactivate(): void {
	// no-op
}

function openRevision(context: vscode.ExtensionContext): void {
	const binary = resolveBinary(context);
	if (!binary) {
		void vscode.window.showErrorMessage(
			"revision: could not find the 'revision' binary. Install it from " +
				'https://github.com/bapatchirag/revision or set "revision.binaryPath".'
		);
		return;
	}

	const cwd = resolveWorkingDirectory();
	const terminal = vscode.window.createTerminal({
		name: 'revision',
		cwd,
		location: vscode.TerminalLocation.Editor,
	});
	terminal.show();
	terminal.sendText(shellQuote(binary), true);
}

// resolveBinary implements the resolution order:
// 1. `revision.binaryPath` setting → 2. bundled binary → 3. `revision` on PATH.
function resolveBinary(context: vscode.ExtensionContext): string | undefined {
	const config = vscode.workspace.getConfiguration('revision');

	const override = (config.get<string>('binaryPath') ?? '').trim();
	if (override !== '') {
		return existingFile(override);
	}

	const exe = process.platform === 'win32' ? 'revision.exe' : 'revision';
	const bundled = existingFile(path.join(context.extensionPath, 'bin', exe));
	if (bundled) {
		ensureExecutable(bundled);
		return bundled;
	}

	return lookupOnPath(exe);
}

function existingFile(p: string): string | undefined {
	try {
		if (fs.statSync(p).isFile()) {
			return p;
		}
	} catch {
		// not found
	}
	return undefined;
}

function ensureExecutable(p: string): void {
	if (process.platform === 'win32') {
		return;
	}
	try {
		fs.chmodSync(p, 0o755);
	} catch {
		// best effort — the file may already be executable
	}
}

function lookupOnPath(exe: string): string | undefined {
	const envPath = process.env.PATH ?? '';
	for (const dir of envPath.split(path.delimiter)) {
		if (dir === '') {
			continue;
		}
		const candidate = existingFile(path.join(dir, exe));
		if (candidate) {
			return candidate;
		}
	}
	return undefined;
}

function resolveWorkingDirectory(): string | undefined {
	const configured = (vscode.workspace
		.getConfiguration('revision')
		.get<string>('workingDirectory') ?? '').trim();
	if (configured !== '') {
		return configured;
	}
	const folders = vscode.workspace.workspaceFolders;
	if (folders && folders.length > 0) {
		return folders[0].uri.fsPath;
	}
	return undefined;
}

function shellQuote(p: string): string {
	return /\s/.test(p) ? `"${p}"` : p;
}
