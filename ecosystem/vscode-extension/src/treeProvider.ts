import * as vscode from 'vscode';
import * as cp from 'child_process';

export class TartarusTreeProvider implements vscode.TreeDataProvider<SandboxItem> {
    private _onDidChangeTreeData: vscode.EventEmitter<SandboxItem | undefined | null | void> = new vscode.EventEmitter<SandboxItem | undefined | null | void>();
    readonly onDidChangeTreeData: vscode.Event<SandboxItem | undefined | null | void> = this._onDidChangeTreeData.event;

    constructor() { }

    refresh(): void {
        this._onDidChangeTreeData.fire();
    }

    getTreeItem(element: SandboxItem): vscode.TreeItem {
        return element;
    }

    getChildren(element?: SandboxItem): Thenable<SandboxItem[]> {
        if (element) {
            return Promise.resolve([]);
        }

        return this.getSandboxes();
    }

    private getSandboxes(): Promise<SandboxItem[]> {
        return new Promise((resolve) => {
            const tartarusPath = vscode.workspace.getConfiguration('tartarus').get('executablePath', 'tartarus');
            // Assuming 'tartarus ps -o json' returns a JSON array of sandboxes
            cp.exec(`${tartarusPath} ps -o json`, (err: cp.ExecException | null, stdout: string, stderr: string) => {
                if (err) {
                    vscode.window.showErrorMessage(`Failed to list sandboxes: ${stderr}`);
                    resolve([]);
                    return;
                }

                try {
                    const sandboxes = JSON.parse(stdout);
                    const items = sandboxes.map((s: any) => {
                        return new SandboxItem(
                            s.ID, // Or s.Name if preferred
                            s.Status,
                            s.Image,
                            vscode.TreeItemCollapsibleState.None
                        );
                    });
                    resolve(items);
                } catch (e) {
                    // Fallback or empty if JSON parse fails (or if output handles empty differently)
                    resolve([]);
                }
            });
        });
    }
}

export class SandboxItem extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public readonly status: string,
        public readonly image: string,
        public readonly collapsibleState: vscode.TreeItemCollapsibleState
    ) {
        super(label, collapsibleState);
        this.tooltip = `${this.label} (${this.image}) - ${this.status}`;
        this.description = this.status;
        this.contextValue = 'sandbox';

        // Simple icon selection based on status
        if (this.status.toLowerCase() === 'running') {
            this.iconPath = new vscode.ThemeIcon('play');
        } else if (this.status.toLowerCase() === 'stopped') {
            this.iconPath = new vscode.ThemeIcon('stop');
        } else {
            this.iconPath = new vscode.ThemeIcon('question');
        }
    }
}
