package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"github.com/isletnet/uptp/agent"
)

func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

type AppModel struct {
	walk.TableModelBase
	apps    []*agent.App
	current struct {
		IsRunning bool
	}
}

func (m *AppModel) SetCurrentIndex(index int) {
	if index >= 0 && index < len(m.apps) {
		m.current.IsRunning = m.apps[index].Running
	} else {
		m.current.IsRunning = false
	}
}

// 刷新应用列表
func (m *AppModel) refreshApps(logView *walk.TextEdit) error {
	apps := agent.GetApps()
	// 转换为指针切片
	appPtrs := make([]*agent.App, len(apps))
	for i := range apps {
		appPtrs[i] = &apps[i]
	}
	m.apps = appPtrs
	m.PublishRowsReset()
	return nil
}

func (m *AppModel) RowCount() int {
	return len(m.apps)
}

func (m *AppModel) Value(row, col int) interface{} {
	app := m.apps[row]
	switch col {
	case 0:
		return app.Name
	case 1:
		return app.PeerName // 显示peername
	case 2:
		return app.Network
	case 3:
		return app.LocalIP
	case 4:
		return app.LocalPort
	case 5:
		if app.TargetAddr == "" || app.TargetPort == 0 {
			return ""
		}
		return fmt.Sprintf("%s:%d", app.TargetAddr, app.TargetPort)
	case 6:
		if app.Running {
			return "运行中"
		}
		return "已停止"
	case 7:
		if app.Err != "" {
			return app.Err
		}
		return ""
	}
	return ""
}

func main() {
	var mw *walk.MainWindow
	var tv *walk.TableView
	var logView *walk.TextEdit

	// 初始化应用模型
	model := &AppModel{}

	// 获取工作目录
	exePath, _ := os.Executable()
	workDir := filepath.Dir(exePath)

	// 启动agent
	if err := agent.Start(workDir); err != nil {
		walk.MsgBox(nil, "错误", fmt.Sprintf("启动Agent失败: %v", err), walk.MsgBoxIconError)
		return
	}

	// 获取应用列表
	if err := model.refreshApps(nil); err != nil {
		walk.MsgBox(nil, "错误", fmt.Sprintf("获取应用列表失败: %v", err), walk.MsgBoxIconError)
		return
	}

	MainWindow{
		AssignTo: &mw,
		Title:    "UPTP Agent",
		MinSize:  Size{800, 600},
		Layout:   VBox{},
		Children: []Widget{
			TableView{
				AssignTo: &tv,
				OnCurrentIndexChanged: func() {
					model.SetCurrentIndex(tv.CurrentIndex())
				},
				Columns: []TableViewColumn{
					{Title: "应用名称", Width: 120},
					{Title: "Peer名称", Width: 120},
					{Title: "协议", Width: 60},
					{Title: "本地IP", Width: 100},
					{Title: "本地端口", Width: 80},
					{Title: "目标地址", Width: 150},
					{Title: "状态", Width: 80},
					{Title: "错误信息", Width: 200},
				},
				ColumnsOrderable: true,
				OnItemActivated: func() {
					idx := tv.CurrentIndex()
					if idx < 0 || idx >= len(model.apps) {
						return
					}
					app := model.apps[idx]

					var (
						dlg      *walk.Dialog
						acceptPB *walk.PushButton
						db       *walk.DataBinder
					)

					type AppForm struct {
						Name       string
						PeerID     string
						ResID      string
						Network    string
						TargetAddr string
						TargetPort int
						LocalIP    string
						LocalPort  int
						Running    bool
					}

					form := &AppForm{
						Name:       app.Name,
						PeerID:     app.PeerID,
						ResID:      strconv.FormatUint(app.ResID, 10),
						Network:    app.Network,
						TargetAddr: app.TargetAddr,
						TargetPort: app.TargetPort,
						LocalIP:    app.LocalIP,
						LocalPort:  app.LocalPort,
						Running:    app.Running,
					}

					if result, err := (Dialog{
						AssignTo: &dlg,
						Title:    "编辑应用",
						MinSize:  Size{400, 300},
						Layout:   VBox{},
						DataBinder: DataBinder{
							AssignTo:   &db,
							DataSource: form,
							AutoSubmit: true,
						},
						Children: []Widget{
							Label{Text: "应用名称:"},
							LineEdit{
								Text: Bind("Name"),
							},
							Label{Text: "PeerID:"},
							LineEdit{
								Text:      app.PeerID,
								ReadOnly:  true,
								TextColor: walk.RGB(100, 100, 100),
							},
							Label{Text: "ResID:"},
							LineEdit{
								Text:      strconv.FormatUint(app.ResID, 10),
								ReadOnly:  true,
								TextColor: walk.RGB(100, 100, 100),
							},
							Label{Text: "协议(tcp/udp):"},
							LineEdit{
								Text: Bind("Network"),
							},
							Label{Text: "本地IP:"},
							LineEdit{
								Text: Bind("LocalIP"),
							},
							Label{Text: "本地端口:"},
							NumberEdit{
								Value: Bind("LocalPort"),
							},
							Label{Text: "目标地址:"},
							LineEdit{
								Text: Bind("TargetAddr"),
							},
							Label{Text: "目标端口:"},
							NumberEdit{
								Value: Bind("TargetPort"),
							},
							CheckBox{
								Text:    "启动应用",
								Checked: Bind("Running"),
							},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									PushButton{
										AssignTo: &acceptPB,
										Text:     "确定",
										OnClicked: func() {
											if err := db.Submit(); err != nil {
												logView.AppendText(fmt.Sprintf("表单验证失败: %v\r\n", err))
												return
											}
											dlg.Accept()
										},
									},
									PushButton{
										Text: "取消",
										OnClicked: func() {
											dlg.Cancel()
										},
									},
								},
							},
						},
					}).Run(mw); err != nil {
						logView.AppendText(fmt.Sprintf("对话框错误: %v\r\n", err))
						return
					} else if result != walk.DlgCmdOK {
						return
					}

					// 更新应用信息(PeerID和ResID保持不变)
					updatedApp := &agent.App{
						ID:         app.ID, // 保留原ID
						Name:       form.Name,
						PeerID:     app.PeerID, // 使用原PeerID
						ResID:      app.ResID,  // 使用原ResID
						Network:    form.Network,
						TargetAddr: form.TargetAddr,
						TargetPort: form.TargetPort,
						LocalIP:    form.LocalIP,
						LocalPort:  form.LocalPort,
						Running:    form.Running,
					}

					// 调用agent接口更新应用
					if err := agent.UpdateApp(updatedApp); err != nil {
						logView.AppendText(fmt.Sprintf("更新应用失败: %v\r\n", err))
					}

					// 无论成功与否都刷新应用列表
					if err := model.refreshApps(logView); err != nil {
						logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
					}
					tv.SetModel(model)
					logView.AppendText(fmt.Sprintf("成功更新应用: %s\r\n", updatedApp.Name))
				},
				ContextMenuItems: []MenuItem{
					Action{
						Text:    "启动应用",
						Visible: Bind("current.IsRunning == false"),
						OnTriggered: func() {
							idx := tv.CurrentIndex()
							if idx < 0 || idx >= len(model.apps) {
								return
							}
							app := model.apps[idx]
							app.Running = true
							if err := agent.UpdateApp(app); err != nil {
								logView.AppendText(fmt.Sprintf("启动应用失败: %v\r\n", err))
							}
							if err := model.refreshApps(logView); err != nil {
								logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
							}
						},
					},
					Action{
						Text:    "停止应用",
						Visible: Bind("current.IsRunning == true"),
						OnTriggered: func() {
							idx := tv.CurrentIndex()
							if idx < 0 || idx >= len(model.apps) {
								return
							}
							app := model.apps[idx]
							app.Running = false
							if err := agent.UpdateApp(app); err != nil {
								logView.AppendText(fmt.Sprintf("停止应用失败: %v\r\n", err))
							}
							if err := model.refreshApps(logView); err != nil {
								logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
							}
						},
					},
					Separator{},
					Action{
						Text: "删除应用",
						OnTriggered: func() {
							idx := tv.CurrentIndex()
							if idx < 0 || idx >= len(model.apps) {
								logView.AppendText("请先选择要删除的应用\r\n")
								return
							}
							app := model.apps[idx]
							if err := agent.DelApp(app); err != nil {
								logView.AppendText(fmt.Sprintf("删除应用失败: %v\r\n", err))
							} else if err := model.refreshApps(logView); err != nil {
								logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
							}
							tv.SetModel(model)
						},
					},
				},
				Model: model,
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "添加应用",
						OnClicked: func() {
							var (
								dlg      *walk.Dialog
								acceptPB *walk.PushButton
								db       *walk.DataBinder
							)

							// 捕获外部变量
							model := model
							tv := tv
							logView := logView

							type AppForm struct {
								Name       string
								PeerID     string
								ResID      string
								Network    string
								TargetAddr string
								TargetPort int
								LocalIP    string
								LocalPort  int
								Running    bool
							}

							form := &AppForm{
								LocalIP: "127.0.0.1",
								Running: true,
							}

							if result, err := (Dialog{
								AssignTo: &dlg,
								Title:    "添加应用",
								MinSize:  Size{400, 300},
								Layout:   VBox{},
								DataBinder: DataBinder{
									AssignTo:   &db,
									DataSource: form,
									AutoSubmit: true,
									// OnSubmitted: func() {
									// 	logView.AppendText(fmt.Sprintf("表单已提交: %+v\r\n", form))
									// },
								},
								Children: []Widget{
									Label{Text: "应用名称:"},
									LineEdit{
										Text: Bind("Name"),
									},
									Label{Text: "PeerID:"},
									LineEdit{
										Text: Bind("PeerID"),
									},
									Label{Text: "ResID:"},
									LineEdit{
										Text: Bind("ResID"),
									},
									Label{Text: "协议(tcp/udp):"},
									LineEdit{
										Text: Bind("Network"),
									},
									Label{Text: "本地IP:"},
									LineEdit{
										Text: Bind("LocalIP"),
									},
									Label{Text: "本地端口:"},
									NumberEdit{
										Value: Bind("LocalPort"),
									},
									Label{Text: "目标地址:"},
									LineEdit{
										Text: Bind("TargetAddr"),
									},
									Label{Text: "目标端口:"},
									NumberEdit{
										Value: Bind("TargetPort"),
									},
									CheckBox{
										Text:    "启动应用",
										Checked: Bind("Running"),
									},
									Composite{
										Layout: HBox{},
										Children: []Widget{
											PushButton{
												AssignTo: &acceptPB,
												Text:     "确定",
												OnClicked: func() {
													dlg.Accept()
												},
											},
											PushButton{
												Text: "取消",
												OnClicked: func() {
													dlg.Cancel()
												},
											},
										},
									},
								},
							}).Run(mw); err != nil {
								logView.AppendText(fmt.Sprintf("对话框错误: %v\r\n", err))
								return
							} else if result != walk.DlgCmdOK {
								logView.AppendText("用户取消了添加应用操作\r\n")
								return
							}

							// 提交并验证表单数据
							logView.AppendText(fmt.Sprintf("提交前表单数据: %+v\r\n", form))
							if err := db.Submit(); err != nil {
								logView.AppendText(fmt.Sprintf("表单验证失败: %v\r\n", err))
								return
							}
							logView.AppendText(fmt.Sprintf("提交后表单数据: %+v\r\n", form))

							// 创建新应用实例
							app := &agent.App{
								Name:       form.Name,
								PeerID:     form.PeerID,
								ResID:      parseUint64(form.ResID),
								Network:    form.Network,
								TargetAddr: form.TargetAddr,
								TargetPort: form.TargetPort,
								LocalIP:    form.LocalIP,
								LocalPort:  form.LocalPort,
								Running:    form.Running,
							}

							// 调用agent接口添加应用
							if err := agent.AddApp(app); err != nil {
								logView.AppendText(fmt.Sprintf("添加应用失败: %v\r\n", err))
							}

							// 无论成功与否都刷新应用列表
							if err := model.refreshApps(logView); err != nil {
								logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
							}
							tv.SetModel(model)
							logView.AppendText(fmt.Sprintf("成功添加应用: %s (PeerID: %s, ResID: %d)\r\n",
								app.Name, app.PeerID, app.ResID))
						},
					},
					PushButton{
						Text: "编辑应用",
						OnClicked: func() {
							idx := tv.CurrentIndex()
							if idx < 0 || idx >= len(model.apps) {
								logView.AppendText("请先选择要编辑的应用\r\n")
								return
							}
							app := model.apps[idx]

							var (
								dlg      *walk.Dialog
								acceptPB *walk.PushButton
								db       *walk.DataBinder
							)

							type AppForm struct {
								Name       string
								PeerID     string
								ResID      string
								Network    string
								TargetAddr string
								TargetPort int
								LocalIP    string
								LocalPort  int
								Running    bool
							}

							form := &AppForm{
								Name:       app.Name,
								PeerID:     app.PeerID,
								ResID:      strconv.FormatUint(app.ResID, 10),
								Network:    app.Network,
								TargetAddr: app.TargetAddr,
								TargetPort: app.TargetPort,
								LocalIP:    app.LocalIP,
								LocalPort:  app.LocalPort,
								Running:    app.Running,
							}

							if result, err := (Dialog{
								AssignTo: &dlg,
								Title:    "编辑应用",
								MinSize:  Size{400, 300},
								Layout:   VBox{},
								DataBinder: DataBinder{
									AssignTo:   &db,
									DataSource: form,
									AutoSubmit: true,
								},
								Children: []Widget{
									Label{Text: "应用名称:"},
									LineEdit{
										Text: Bind("Name"),
									},
									Label{Text: "PeerID:"},
									LineEdit{
										Text: Bind("PeerID"),
									},
									Label{Text: "ResID:"},
									LineEdit{
										Text: Bind("ResID"),
									},
									Label{Text: "协议(tcp/udp):"},
									LineEdit{
										Text: Bind("Network"),
									},
									Label{Text: "目标地址:"},
									LineEdit{
										Text: Bind("TargetAddr"),
									},
									Label{Text: "目标端口:"},
									NumberEdit{
										Value: Bind("TargetPort"),
									},
									Label{Text: "本地IP:"},
									LineEdit{
										Text: Bind("LocalIP"),
									},
									Label{Text: "本地端口:"},
									NumberEdit{
										Value: Bind("LocalPort"),
									},
									CheckBox{
										Text:    "启动应用",
										Checked: Bind("Running"),
									},
									Composite{
										Layout: HBox{},
										Children: []Widget{
											PushButton{
												AssignTo: &acceptPB,
												Text:     "确定",
												OnClicked: func() {
													if err := db.Submit(); err != nil {
														logView.AppendText(fmt.Sprintf("表单验证失败: %v\r\n", err))
														return
													}
													dlg.Accept()
												},
											},
											PushButton{
												Text: "取消",
												OnClicked: func() {
													dlg.Cancel()
												},
											},
										},
									},
								},
							}).Run(mw); err != nil {
								logView.AppendText(fmt.Sprintf("对话框错误: %v\r\n", err))
								return
							} else if result != walk.DlgCmdOK {
								return
							}

							// 更新应用信息(PeerID和ResID保持不变)
							updatedApp := &agent.App{
								Name:       form.Name,
								PeerID:     app.PeerID, // 使用原PeerID
								ResID:      app.ResID,  // 使用原ResID
								Network:    form.Network,
								TargetAddr: form.TargetAddr,
								TargetPort: form.TargetPort,
								LocalIP:    form.LocalIP,
								LocalPort:  form.LocalPort,
								Running:    form.Running,
							}

							// 调用agent接口更新应用
							if err := agent.AddApp(updatedApp); err != nil {
								logView.AppendText(fmt.Sprintf("更新应用失败: %v\r\n", err))
								return
							}

							// 刷新应用列表
							if err := model.refreshApps(logView); err != nil {
								logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
								return
							}
							tv.SetModel(model)
							logView.AppendText(fmt.Sprintf("成功更新应用: %s\r\n", updatedApp.Name))
						},
					},
					PushButton{
						Text: "删除应用",
						OnClicked: func() {
							idx := tv.CurrentIndex()
							if idx < 0 || idx >= len(model.apps) {
								logView.AppendText("请先选择要删除的应用\r\n")
								return
							}
							app := model.apps[idx]
							if err := agent.DelApp(app); err != nil {
								logView.AppendText(fmt.Sprintf("删除应用失败: %v\r\n", err))
							} else if err := model.refreshApps(logView); err != nil {
								logView.AppendText(fmt.Sprintf("刷新应用列表失败: %v\r\n", err))
							}
							tv.SetModel(model)
						},
					},
				},
			},
			TextEdit{
				AssignTo: &logView,
				ReadOnly: true,
				VScroll:  true,
			},
		},
	}.Run()
}
