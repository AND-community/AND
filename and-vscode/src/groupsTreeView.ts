import * as vscode from "vscode";
import { listGroups, listProjects } from "./groupProjectStore";
import { Grup, Proje } from "./types";

export class GroupItem extends vscode.TreeItem {
  contextValue = "deltaxGroup";
  constructor(readonly grup: Grup) {
    super(grup.ad, vscode.TreeItemCollapsibleState.Expanded);
    this.description = `${grup.uyeler.length} üye`;
    this.iconPath = new vscode.ThemeIcon("organization");
    this.tooltip = `Kurucu: ${grup.kurucu_ad}`;
  }
}

export class ProjectItem extends vscode.TreeItem {
  contextValue = "deltaxProject";
  constructor(readonly proje: Proje, isActive: boolean) {
    super(proje.ad, vscode.TreeItemCollapsibleState.None);
    this.description = proje.aktif_dal;
    this.iconPath = new vscode.ThemeIcon(isActive ? "folder-active" : "folder");
    this.tooltip = isActive ? "Şu anda VS Code'da açık" : "Açmak için tıklayın";
    this.command = { command: "deltax.openProject", title: "Projeyi Aç", arguments: [this] };
  }
}

type Node = GroupItem | ProjectItem;

export class GroupsTreeProvider implements vscode.TreeDataProvider<Node> {
  private readonly changeEmitter = new vscode.EventEmitter<Node | undefined | void>();
  readonly onDidChangeTreeData = this.changeEmitter.event;

  constructor(private readonly getRoot: () => string, private readonly activeProjectId: () => string | undefined) {}

  refresh(): void {
    this.changeEmitter.fire();
  }

  getTreeItem(element: Node): vscode.TreeItem {
    return element;
  }

  getChildren(element?: Node): Node[] {
    const root = this.getRoot();
    if (!element) {
      return listGroups(root).map((g) => new GroupItem(g));
    }
    if (element instanceof GroupItem) {
      return listProjects(root, element.grup.id).map(
        (p) => new ProjectItem(p, p.id === this.activeProjectId())
      );
    }
    return [];
  }
}
