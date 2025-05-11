// API 基础 URL
const API_BASE_URL = '/portmap';

// 模态框实例
let resourceModal;

// 页面加载完成后初始化
document.addEventListener('DOMContentLoaded', function() {
    resourceModal = new bootstrap.Modal(document.getElementById('resourceModal'));
    loadResources();
});

// 加载资源列表
async function loadResources() {
    try {
        const response = await fetch(`${API_BASE_URL}/list_resources`);
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
        tr.innerHTML = '<td colspan="8" class="text-center">暂无数据</td>';
        tbody.appendChild(tr);
        return;
    }

    resources.forEach(resource => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>${resource.id.toString()}</td>
            <td>${resource.name}</td>
            <td>${resource.network}</td>
            <td>${resource.target_addr}</td>
            <td>${resource.target_port}</td>
            <td>${resource.local_ip || '-'}</td>
            <td>${resource.local_port || '-'}</td>
            <td class="action-buttons">
                <div class="btn-group btn-group-sm">
                    <button class="btn btn-outline-primary" onclick="editResource('${resource.id.toString()}')">
                        <i class="bi bi-pencil"></i>
                    </button>
                    <button class="btn btn-outline-danger" onclick="deleteResource('${resource.id.toString()}')">
                        <i class="bi bi-trash"></i>
                    </button>
                </div>
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
        const response = await fetch(`${API_BASE_URL}/get_resource/${id}`);
        const data = await response.json();
        
        if (data.code === 0) {
            const resource = data.data;
            document.getElementById('modalTitle').textContent = '编辑资源';
            document.getElementById('resourceId').value = resource.id.toString();
            document.getElementById('name').value = resource.name;
            document.getElementById('network').value = resource.network;
            document.getElementById('targetAddr').value = resource.target_addr;
            document.getElementById('targetPort').value = resource.target_port;
            document.getElementById('localIp').value = resource.local_ip || '';
            document.getElementById('localPort').value = resource.local_port || '';
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
        local_ip: document.getElementById('localIp').value || '',
        local_port: document.getElementById('localPort').value ? parseInt(document.getElementById('localPort').value) : 0
    };

    try {
        const url = resourceId ? `${API_BASE_URL}/update_resource` : `${API_BASE_URL}/add_resource`;
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
        const response = await fetch(`${API_BASE_URL}/delete_resource`, {
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

// 显示错误信息
function showError(message) {
    alert(message);
} 