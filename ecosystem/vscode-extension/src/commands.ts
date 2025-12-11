import * as vscode from 'vscode';
import * as cp from 'child_process';
import * as path from 'path';

export function registerCommands(context: vscode.ExtensionContext) {
    context.subscriptions.push(
        vscode.commands.registerCommand('tartarus.initTemplate', initTemplate),
        vscode.commands.registerCommand('tartarus.run', runSandbox),
        vscode.commands.registerCommand('tartarus.logs', streamLogs),
        vscode.commands.registerCommand('tartarus.exec', execShell)
    );
}

function getTartarusPath(): string {
    return vscode.workspace.getConfiguration('tartarus').get('executablePath', 'tartarus');
}

async function initTemplate() {
    const term = vscode.window.createTerminal('Tartarus Init');
    term.show();
    term.sendText(`${getTartarusPath()} init template`);
}

async function runSandbox() {
    const image = await vscode.window.showInputBox({
        prompt: 'Enter sandbox image (e.g., ubuntu:latest)',
        placeHolder: 'ubuntu:latest'
    });

    if (!image) {
        return;
    }

    const name = await vscode.window.showInputBox({
        prompt: 'Enter sandbox name (optional)',
        placeHolder: 'my-sandbox'
    });

    const command = `${getTartarusPath()} run --image ${image} ${name ? `--name ${name}` : ''}`;
    const term = vscode.window.createTerminal('Tartarus Run');
    term.show();
    term.sendText(command);
}

// These commands might be called from the tree view context, so they accept an item
async function streamLogs(item?: any) {
    let sandboxName = item?.label;
    if (!sandboxName) {
        sandboxName = await vscode.window.showInputBox({
            prompt: 'Enter sandbox name to stream logs from',
            placeHolder: 'sandbox-id'
        });
    }

    if (!sandboxName) {
        return;
    }

    const term = vscode.window.createTerminal(`Tartarus Logs: ${sandboxName}`);
    term.show();
    term.sendText(`${getTartarusPath()} logs -f ${sandboxName}`);
}

async function execShell(item?: any) {
    let sandboxName = item?.label;
    if (!sandboxName) {
        sandboxName = await vscode.window.showInputBox({
            prompt: 'Enter sandbox name to exec into',
            placeHolder: 'sandbox-id'
        });
    }

    if (!sandboxName) {
        return;
    }

    const term = vscode.window.createTerminal(`Tartarus Shell: ${sandboxName}`);
    term.show();
    term.sendText(`${getTartarusPath()} exec ${sandboxName} /bin/bash`);
}
