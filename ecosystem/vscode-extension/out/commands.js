"use strict";
var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.registerCommands = void 0;
const vscode = require("vscode");
function registerCommands(context) {
    context.subscriptions.push(vscode.commands.registerCommand('tartarus.initTemplate', initTemplate), vscode.commands.registerCommand('tartarus.run', runSandbox), vscode.commands.registerCommand('tartarus.logs', streamLogs), vscode.commands.registerCommand('tartarus.exec', execShell));
}
exports.registerCommands = registerCommands;
function getTartarusPath() {
    return vscode.workspace.getConfiguration('tartarus').get('executablePath', 'tartarus');
}
function initTemplate() {
    return __awaiter(this, void 0, void 0, function* () {
        const term = vscode.window.createTerminal('Tartarus Init');
        term.show();
        term.sendText(`${getTartarusPath()} init template`);
    });
}
function runSandbox() {
    return __awaiter(this, void 0, void 0, function* () {
        const image = yield vscode.window.showInputBox({
            prompt: 'Enter sandbox image (e.g., ubuntu:latest)',
            placeHolder: 'ubuntu:latest'
        });
        if (!image) {
            return;
        }
        const name = yield vscode.window.showInputBox({
            prompt: 'Enter sandbox name (optional)',
            placeHolder: 'my-sandbox'
        });
        const command = `${getTartarusPath()} run --image ${image} ${name ? `--name ${name}` : ''}`;
        const term = vscode.window.createTerminal('Tartarus Run');
        term.show();
        term.sendText(command);
    });
}
// These commands might be called from the tree view context, so they accept an item
function streamLogs(item) {
    return __awaiter(this, void 0, void 0, function* () {
        let sandboxName = item === null || item === void 0 ? void 0 : item.label;
        if (!sandboxName) {
            sandboxName = yield vscode.window.showInputBox({
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
    });
}
function execShell(item) {
    return __awaiter(this, void 0, void 0, function* () {
        let sandboxName = item === null || item === void 0 ? void 0 : item.label;
        if (!sandboxName) {
            sandboxName = yield vscode.window.showInputBox({
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
    });
}
//# sourceMappingURL=commands.js.map