// API 基础 URL
const API_BASE_URL = '/resource';

// 模态框实例
let resourceModal;

// 页面加载完成后初始化
document.addEventListener('DOMContentLoaded', function() {
    resourceModal = new bootstrap.Modal(document.getElementById('resourceModal'));
    gatewayNameModal = new bootstrap.Modal(document.getElementById('gatewayNameModal'));
    loadResources();
    loadGatewayInfo();
});

// 加载资源列表
async function loadResources() {
    try {
        const response = await fetch(`${API_BASE_URL}/list`);
        const data = await response.json();
        
        if (data.code === 0) {
            // 确保 data.data 是数组
            const resources = Array.isArray(data.data) ? data.data : [];
            renderResourceList(resources);
        } else {
            showError('加载资源列表失败：' + data.message);
        }
    } catch (error) {
        showError('加载资源列表失败：' + error.message);
    }
}

// 渲染资源列表
function renderResourceList(resources) {
    const tbody = document.getElementById('resourceList');
    tbody.innerHTML = '';

    if (!resources || resources.length === 0) {
        const tr = document.createElement('tr');
        tr.innerHTML = '<td colspan="6" class="text-center">暂无数据</td>';
        tbody.appendChild(tr);
        return;
    }

    resources.forEach(resource => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td class="d-flex align-items-center gap-1">
                <span>${resource.id}</span>
                <button class="btn btn-sm btn-light" onclick="copyToClipboard('${resource.id}')" title="复制ID">
                    <i class="bi bi-clipboard"></i>
                </button>
            </td>
            <td>${resource.name}</td>
            <td>${resource.network}</td>
            <td>${resource.target_addr}</td>
            <td>${resource.target_port}</td>
            <td>
                <button class="btn btn-sm btn-outline-primary" onclick="editResource('${resource.id.toString()}')">
                    <i class="bi bi-pencil"></i>
                </button>
                <button class="btn btn-sm btn-outline-danger ms-1" onclick="deleteResource('${resource.id.toString()}')">
                    <i class="bi bi-trash"></i>
                </button>
            </td>
        `;
        tbody.appendChild(tr);
    });
}

// 显示添加资源模态框
function showAddModal() {
    document.getElementById('modalTitle').textContent = '添加资源';
    document.getElementById('resourceForm').reset();
    document.getElementById('resourceId').value = '';
    resourceModal.show();
}

// 显示编辑资源模态框
async function editResource(id) {
    try {
        const response = await fetch(`${API_BASE_URL}/get/${id}`);
        const data = await response.json();
        
        if (data.code === 0) {
            const resource = data.data;
            document.getElementById('modalTitle').textContent = '编辑资源';
            document.getElementById('resourceId').value = resource.id.toString();
            document.getElementById('name').value = resource.name;
            document.getElementById('network').value = resource.network;
            document.getElementById('targetAddr').value = resource.target_addr;
            document.getElementById('targetPort').value = resource.target_port;
            resourceModal.show();
        } else {
            showError('获取资源信息失败：' + data.message);
        }
    } catch (error) {
        showError('获取资源信息失败：' + error.message);
    }
}

// 保存资源
async function saveResource() {
    const form = document.getElementById('resourceForm');
    if (!form.checkValidity()) {
        form.reportValidity();
        return;
    }

    const resourceId = document.getElementById('resourceId').value;
    const resource = {
        name: document.getElementById('name').value,
        network: document.getElementById('network').value,
        target_addr: document.getElementById('targetAddr').value,
        target_port: parseInt(document.getElementById('targetPort').value),
    };

    try {
        const url = resourceId ? `${API_BASE_URL}/update` : `${API_BASE_URL}/add`;
        if (resourceId) {
            resource.id = resourceId;
        }

        const response = await fetch(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(resource)
        });

        const data = await response.json();
        if (data.code === 0) {
            resourceModal.hide();
            loadResources();
        } else {
            showError('保存资源失败：' + data.message);
        }
    } catch (error) {
        showError('保存资源失败：' + error.message);
    }
}

// 删除资源
async function deleteResource(id) {
    if (!confirm('确定要删除这个资源吗？')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE_URL}/delete`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ id: id })
        });

        const data = await response.json();
        if (data.code === 0) {
            loadResources();
        } else {
            showError('删除资源失败：' + data.message);
        }
    } catch (error) {
        showError('删除资源失败：' + error.message);
    }
}

// 加载网关信息
async function loadGatewayInfo() {
    try {
        const response = await fetch('/gateway/info');
        const data = await response.json();
        
        if (data.code === 0) {
            document.getElementById('gatewayId').textContent = data.data.p2p_id;
            document.getElementById('gatewayName').textContent = data.data.name || '未设置';
        } else {
            showError('加载网关信息失败：' + data.message);
        }
    } catch (error) {
        showError('加载网关信息失败：' + error.message);
    }
}

// 显示修改网关名称模态框
function showGatewayNameModal() {
    document.getElementById('newGatewayName').value = document.getElementById('gatewayName').textContent;
    gatewayNameModal.show();
}

// 更新网关名称
async function updateGatewayName() {
    const newName = document.getElementById('newGatewayName').value.trim();
    if (!newName) {
        alert('请输入网关名称');
        return;
    }

    try {
        const response = await fetch('/gateway/name', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ name: newName })
        });

        const data = await response.json();
        if (data.code === 0) {
            document.getElementById('gatewayName').textContent = newName;
            gatewayNameModal.hide();
        } else {
            showError('更新网关名称失败：' + data.message);
        }
    } catch (error) {
        showError('更新网关名称失败：' + error.message);
    }
}

// 通用复制到剪贴板函数
function copyToClipboard(text) {
    if (!text) {
        showError('没有可复制的内容');
        return;
    }

    // 创建临时textarea元素用于复制
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    document.body.appendChild(textarea);
    textarea.select();

    try {
        const successful = document.execCommand('copy');
        if (successful) {
            const toast = new bootstrap.Toast(document.getElementById('copyToast'));
            toast.show();
        } else {
            showError('复制失败，请手动复制');
        }
    } catch (err) {
        showError('复制失败: ' + err);
    } finally {
        document.body.removeChild(textarea);
    }
}

// 复制网关ID到剪贴板
function copyGatewayId() {
    const gatewayId = document.getElementById('gatewayId').textContent.trim();
    copyToClipboard(gatewayId);
}

// 显示错误信息
function showError(message) {
    alert(message);
}