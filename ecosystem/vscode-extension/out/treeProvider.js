"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SandboxItem = exports.TartarusTreeProvider = void 0;
const vscode = require("vscode");
const cp = require("child_process");
class TartarusTreeProvider {
    constructor() {
        this._onDidChangeTreeData = new vscode.EventEmitter();
        this.onDidChangeTreeData = this._onDidChangeTreeData.event;
    }
    refresh() {
        this._onDidChangeTreeData.fire();
    }
    getTreeItem(element) {
        return element;
    }
    getChildren(element) {
        if (element) {
            return Promise.resolve([]);
        }
        return this.getSandboxes();
    }
    getSandboxes() {
        return new Promise((resolve) => {
            const tartarusPath = vscode.workspace.getConfiguration('tartarus').get('executablePath', 'tartarus');
            // Assuming 'tartarus ps -o json' returns a JSON array of sandboxes
            cp.exec(`${tartarusPath} ps -o json`, (err, stdout, stderr) => {
                if (err) {
                    vscode.window.showErrorMessage(`Failed to list sandboxes: ${stderr}`);
                    resolve([]);
                    return;
                }
                try {
                    const sandboxes = JSON.parse(stdout);
                    const items = sandboxes.map((s) => {
                        return new SandboxItem(s.ID, // Or s.Name if preferred
                        s.Status, s.Image, vscode.TreeItemCollapsibleState.None);
                    });
                    resolve(items);
                }
                catch (e) {
                    // Fallback or empty if JSON parse fails (or if output handles empty differently)
                    resolve([]);
                }
            });
        });
    }
}
exports.TartarusTreeProvider = TartarusTreeProvider;
class SandboxItem extends vscode.TreeItem {
    constructor(label, status, image, collapsibleState) {
        super(label, collapsibleState);
        this.label = label;
        this.status = status;
        this.image = image;
        this.collapsibleState = collapsibleState;
        this.tooltip = `${this.label} (${this.image}) - ${this.status}`;
        this.description = this.status;
        this.contextValue = 'sandbox';
        // Simple icon selection based on status
        if (this.status.toLowerCase() === 'running') {
            this.iconPath = new vscode.ThemeIcon('play');
        }
        else if (this.status.toLowerCase() === 'stopped') {
            this.iconPath = new vscode.ThemeIcon('stop');
        }
        else {
            this.iconPath = new vscode.ThemeIcon('question');
        }
    }
}
exports.SandboxItem = SandboxItem;
//# sourceMappingURL=treeProvider.js.map