const $ = s => document.querySelector(s), $$ = s => [...document.querySelectorAll(s)];
let token = localStorage.getItem('bookly_token') || '',
    currentUser = JSON.parse(localStorage.getItem('bookly_user') || 'null'), authMode = 'login', books = [], users = [],
    page = 'library', selectedUserId = null;
const bookFields = `id userId title author status isFavorite pageCount currentPage quotes price review rating createdAt updatedAt`;
const userFields = `id name nick location birthDate photo createdAt updatedAt`;

async function request(path, body) {
    const r = await fetch(path, {
        method: 'POST',
        headers: {'Content-Type': 'application/json', ...(token ? {Authorization: 'Bearer ' + token} : {})},
        body: JSON.stringify(body)
    });
    return readBody(r)
}

async function readBody(r) {
    const text = await r.text();
    let data = null;
    try {
        data = text ? JSON.parse(text) : {}
    } catch (_) {
    }
    if (r.ok) return data ?? {};
    const plain = (data && data.error) || (data === null && text.trim().length < 200 ? text.trim() : '');
    const byStatus = {
        400: plain || 'The request was not valid. Please check your input.',
        401: r.url.includes('/auth/login') ? 'Incorrect email or password.' : 'Unauthorized: your session has expired, please sign in again.',
        403: 'You do not have permission to do that.',
        404: 'Not found.',
        409: plain || 'This email or nickname is already registered.',
        413: 'That request is too large.',
        429: 'Too many attempts. Please wait a minute and try again.'
    };
    throw Error(byStatus[r.status] || (r.status >= 500 ? 'Something went wrong on our side. Please try again later.' : plain || 'Request failed.'))
}

async function getJSON(path) {
    const r = await fetch(path, {headers: token ? {Authorization: 'Bearer ' + token} : {}});
    return readBody(r)
}

async function gql(query) {
    const r = await fetch('/query', {
        method: 'POST',
        headers: {'Content-Type': 'application/json', ...(token ? {Authorization: 'Bearer ' + token} : {})},
        body: JSON.stringify({query})
    });
    const j = await readBody(r);
    if (j.errors?.length) throw Error(j.errors.map(e => e.message).join('\n'));
    return j.data
}

function notice(msg, bad = false) {
    const n = $('#notice');
    n.textContent = msg;
    n.classList.remove('hidden');
    n.style.background = bad ? '#f8e9e7' : '#e9efe5';
    n.style.color = bad ? '#a0443d' : '#456044';
    setTimeout(() => n.classList.add('hidden'), 4000)
}

function escapeHTML(s = '') {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
    }[c]))
}

function setAvatar(u) {
    const el = $('#avatar');
    const letter = ((u && u.name) || 'B')[0].toUpperCase();
    el.textContent = letter;
    el.style.backgroundImage = '';
    if (u && u.photo) {
        const img = new Image();
        img.onload = () => {
            el.textContent = '';
            el.style.backgroundImage = `url("${u.photo.replace(/"/g, '%22')}")`
        };
        img.referrerPolicy = 'no-referrer';
        img.src = u.photo
    }
}

function setAuth() {
    localStorage.setItem('bookly_token', token);
    localStorage.setItem('bookly_user', JSON.stringify(currentUser));
    $('#authView').classList.add('hidden');
    $('#appView').classList.remove('hidden');
    $('#sideName').textContent = currentUser.name;
    $('#sideRole').textContent = currentUser.role;
    $('#rolePill').textContent = currentUser.role;
    setAvatar(currentUser);
    const isAdmin = currentUser.role === 'ADMIN';
    $$('.admin-only').forEach(x => x.classList.toggle('hidden', !isAdmin));
    $$('.user-only').forEach(x => x.classList.toggle('hidden', isAdmin));
    $('#navLibraryLabel').textContent = isAdmin ? 'All libraries' : 'My library';
    $('#brandSub').textContent = isAdmin ? 'ADMIN PANEL' : 'PERSONAL LIBRARY';
    if (isAdmin && ['favorites', 'statistics'].includes(page)) page = 'library';
    $('#addBookBtn').classList.toggle('hidden', !['library', 'favorites'].includes(page));
    loadAll();
    gql(`query { user(id:"${currentUser.id}") { ${userFields} } }`).then(d => {
        if (d.user) {
            currentUser.photo = d.user.photo;
            localStorage.setItem('bookly_user', JSON.stringify(currentUser));
            setAvatar(currentUser)
        }
    }).catch(() => {
    })
}

function logout() {
    token = '';
    currentUser = null;
    localStorage.removeItem('bookly_token');
    localStorage.removeItem('bookly_user');
    $('#appView').classList.add('hidden');
    $('#authView').classList.remove('hidden')
}

$$('.tab').forEach(b => b.onclick = () => {
    authMode = b.dataset.mode;
    $$('.tab').forEach(x => x.classList.toggle('active', x === b));
    $('#nameFields').classList.toggle('hidden', authMode === 'login');
    $('#authTitle').textContent = authMode === 'login' ? 'Welcome back' : 'Create your account';
    $('#authSubmit').textContent = authMode === 'login' ? 'Sign in' : 'Create account';
    $('#authError').textContent = ''
});
$('#authForm').onsubmit = async e => {
    e.preventDefault();
    const f = new FormData(e.target), body = Object.fromEntries(f);
    try {
        const d = await request('/auth/' + (authMode === 'login' ? 'login' : 'register'), body);
        token = d.token;
        currentUser = d.user;
        setAuth()
    } catch (err) {
        $('#authError').textContent = err.message
    }
};
$('#logout').onclick = logout;

async function loadAll() {
    try {
        if (currentUser.role === 'ADMIN') {
            const a = await gql(`query { users { ${userFields} } }`);
            users = a.users;
        }
        const d = await gql(`query { books { ${bookFields} } myReadingStatistics { totalCompletedBooks currentlyReading wantToRead readingProgress yearlyReadingGoal completedThisYear yearlyGoalProgress } }`);
        books = d.books;
        renderBooks();
        renderStats(d.myReadingStatistics);
        if (currentUser.role === 'ADMIN') renderUsers();
    } catch (e) {
        notice(e.message, true);
        if (/authentication|required|unauthorized/i.test(e.message)) logout()
    }
}

function renderBooks() {
    let list = [...books], search = $('#searchInput').value.trim().toLowerCase(), status = $('#statusFilter').value;
    if (page === 'favorites') list = list.filter(b => b.isFavorite);
    if (status) list = list.filter(b => b.status === status);
    if (search) list = list.filter(b => (b.title + ' ' + b.author).toLowerCase().includes(search));
    const sort = $('#sortFilter').value;
    if (sort === 'title') list.sort((a, b) => a.title.localeCompare(b.title));
    if (sort === 'rating') list.sort((a, b) => b.rating - a.rating);
    if (sort === 'newest') list.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    $('#allCount').textContent = books.length;
    $('#readingCount').textContent = books.filter(b => b.status === 'READING').length;
    $('#completedCount').textContent = books.filter(b => b.status === 'COMPLETED').length;
    $('#wantCount').textContent = books.filter(b => b.status === 'WANT_TO_READ').length;
    $('#shelfTitle').textContent = page === 'favorites' ? 'Favorite books' : status ? ({
        READING: 'Currently reading',
        COMPLETED: 'Completed',
        WANT_TO_READ: 'Want to read'
    }[status]) : 'All books';
    $('#resultCount').textContent = `${list.length} books`;
    $('#bookGrid').innerHTML = list.length ? list.map(b => `<article class="book-card"><div class="book-top"><span class="status ${b.status}">${b.status.replaceAll('_', ' ')}</span><div class="book-actions">${currentUser.role === 'ADMIN' ? '' : `<button title="Toggle favorite" onclick="favoriteBook('${b.id}')">${b.isFavorite ? '♥' : '♡'}</button>`}<button title="Edit" onclick="editBook('${b.id}')">✎</button><button title="Delete" onclick="removeBook('${b.id}')">×</button></div></div><div class="book-cover">${escapeHTML(b.title.slice(0, 1))}</div><h4>${escapeHTML(b.title)}</h4><div class="author">${escapeHTML(b.author)}</div>${currentUser.role === 'ADMIN' ? `<div class="owner-tag">Owner: ${escapeHTML(users.find(u => u.id === b.userId)?.name || 'Unknown user')} · ${escapeHTML(users.find(u => u.id === b.userId)?.nick || b.userId)}</div>` : ''}<div class="book-meta"><span class="muted">${b.rating ? '★'.repeat(b.rating) + '☆'.repeat(5 - b.rating) : 'Not rated'}</span><span class="muted">${b.currentPage != null && b.pageCount ? Math.round(b.currentPage / b.pageCount * 100) + '%' : ''}</span></div></article>`).join('') : '<div class="empty">No books here yet. Add your first book to get started.</div>'
}

function renderStats(s) {
    $('#statCompleted').textContent = s.totalCompletedBooks;
    $('#statReading').textContent = s.currentlyReading;
    $('#statWant').textContent = s.wantToRead;
    $('#statProgress').textContent = Math.round(s.readingProgress) + '%';
    $('#yearDone').textContent = s.completedThisYear;
    $('#goalInput').value = s.yearlyReadingGoal;
    const pct = s.yearlyReadingGoal ? Math.min(100, s.yearlyGoalProgress) : 0;
    $('#goalBar').style.width = pct + '%';
    $('#goalText').textContent = s.yearlyReadingGoal ? `${s.completedThisYear} of ${s.yearlyReadingGoal} books · ${Math.round(pct)}% complete` : 'Set a goal to start tracking your year.'
}

function fillProfile(u) {
    if (!u) return;
    const f = $('#profileForm');
    for (const k of ['name', 'nick', 'location', 'birthDate', 'photo']) f.elements[k].value = u[k] || ''
}

function renderUsers() {
    $('#usersList').innerHTML = '<div class="user-row"><b>Name</b><b>Nickname</b><b>Account ID</b><b>Actions</b></div>' + users.map(u => `<div class="user-row"><span>${escapeHTML(u.name)}</span><span>@${escapeHTML(u.nick)}</span><small>${u.id}</small><span class="user-actions"><button class="secondary" onclick="viewUser('${u.id}')">Manage</button>${u.id === currentUser.id ? '<span class="role-pill">ADMIN</span>' : `<button class="danger" onclick="removeUser('${u.id}')">Delete</button>`}</span></div>`).join('');
}

window.viewUser = id => {
    if (currentUser.role !== 'ADMIN') return;
    const u = users.find(x => x.id === id);
    if (!u) return;
    selectedUserId = id;
    const userBooks = books.filter(b => b.userId === id);
    $('#adminAccountDetail').classList.remove('hidden');
    $('#adminAccountDetail').innerHTML = `<div class="panel admin-detail"><div class="section-heading"><div><span class="eyebrow">ACCOUNT MANAGEMENT</span><h3>${escapeHTML(u.name)} <small class="muted">@${escapeHTML(u.nick)}</small></h3></div><button class="secondary" onclick="closeUserDetail()">Close</button></div><div class="admin-user-grid"><label>Name<input id="adminUserName" value="${escapeHTML(u.name)}"></label><label>Nickname<input id="adminUserNick" value="${escapeHTML(u.nick)}"></label><label>Location<input id="adminUserLocation" value="${escapeHTML(u.location || '')}"></label><label>Birth date<input id="adminUserBirth" type="date" value="${escapeHTML(u.birthDate || '')}"></label><label class="full">Photo URL<input id="adminUserPhoto" value="${escapeHTML(u.photo || '')}"></label></div><button class="primary" onclick="saveManagedUser('${id}')">Save account changes</button><div id="managedAccountMeta" class="managed-meta muted">Loading account details…</div><div class="admin-user-stats"><span><b>${userBooks.length}</b> books in library</span><span><b>${userBooks.filter(b => b.status === 'COMPLETED').length}</b> completed</span><span><b>${userBooks.filter(b => b.status === 'READING').length}</b> reading</span><span><b>${userBooks.filter(b => b.status === 'WANT_TO_READ').length}</b> want to read</span></div><div class="section-heading"><h3>This user's books</h3><button class="primary" onclick="openAdminBook('${id}')">＋ Add book to this account</button></div><div class="managed-books">${userBooks.length ? userBooks.map(b => `<div class="managed-book"><div><b>${escapeHTML(b.title)}</b><small>${escapeHTML(b.author)} · ${b.status.replaceAll('_', ' ')}</small></div><div><button class="secondary" onclick="editBook('${b.id}')">Edit</button><button class="danger" onclick="removeBook('${b.id}')">Delete</button></div></div>`).join('') : '<p class="muted">This account has no books yet.</p>'}</div><div class="admin-goal"><label>Yearly reading goal<input id="adminGoal" type="number" min="0" max="500" value="${Number(u.yearlyReadingGoal || 0)}"></label><button class="secondary" onclick="saveUserGoal('${id}')">Update reading goal</button></div></div>`;
    getJSON('/admin/users/' + id).then(x => {
        if (selectedUserId !== id) return;
        $('#adminGoal').value = x.yearlyReadingGoal;
        $('#managedAccountMeta').innerHTML = `<span>Email: <b>${escapeHTML(x.email || '—')}</b></span><span>Role: <b>${escapeHTML(x.role)}</b></span><span>Created: <b>${escapeHTML(String(x.createdAt || '').slice(0, 10))}</b></span><label>Account role<select id="managedRole"><option value="USER" ${x.role === 'USER' ? 'selected' : ''}>USER</option><option value="ADMIN" ${x.role === 'ADMIN' ? 'selected' : ''}>ADMIN</option></select></label><button class="secondary" onclick="saveManagedRole('${id}')">Update role</button>`
    }).catch(e => notice(e.message, true));
};
window.closeUserDetail = () => {
    $('#adminAccountDetail').classList.add('hidden');
    selectedUserId = null
};
window.saveManagedUser = async id => {
    const q = `mutation { updateUser(id:"${id}",input:{name:${JSON.stringify($('#adminUserName').value)},nick:${JSON.stringify($('#adminUserNick').value)},location:${JSON.stringify($('#adminUserLocation').value)},birthDate:${JSON.stringify($('#adminUserBirth').value)},photo:${JSON.stringify($('#adminUserPhoto').value)}}) { id } }`;
    try {
        await gql(q);
        await loadAll();
        viewUser(id);
        notice('User account updated.')
    } catch (e) {
        notice(e.message, true)
    }
};
window.saveManagedRole = async id => {
    try {
        await request('/admin/users/role', {userId: id, role: $('#managedRole').value});
        await loadAll();
        viewUser(id);
        notice('Role updated. The user must sign in again for the new role to take effect.');
    } catch (e) {
        notice(e.message, true)
    }
};
window.saveUserGoal = async id => {
    try {
        await request('/admin/users/goal', {userId: id, goal: Number($('#adminGoal').value)});
        notice('Reading goal updated.')
    } catch (e) {
        notice(e.message, true)
    }
};

function showPage(p) {
    page = p;
    const section = (p === 'favorites' || p === 'library') ? 'libraryPage' : p + 'Page';
    $$('.page').forEach(x => x.classList.add('hidden'));
    $('#' + section).classList.remove('hidden');
    $$('.nav').forEach(x => x.classList.toggle('active', x.dataset.page === p));
    $('#pageTitle').textContent = ({
        library: currentUser.role === 'ADMIN' ? 'All libraries' : 'My library',
        favorites: 'Favorites',
        statistics: 'Reading statistics',
        users: 'Users',
        profile: 'Profile'
    })[p];
    $('#addBookBtn').classList.toggle('hidden', !['library', 'favorites'].includes(p));
    if (['library', 'favorites'].includes(p)) renderBooks();
    if (p === 'users' && currentUser.role === 'ADMIN') renderUsers();
    if (p === 'profile') gql(`query { user(id:"${currentUser.id}") { ${userFields} } }`).then(d => fillProfile(d.user)).catch(e => notice(e.message, true))
}

$$('.nav').forEach(b => b.onclick = () => showPage(b.dataset.page));
$('#searchInput').oninput = renderBooks;
$('#statusFilter').onchange = renderBooks;
$('#sortFilter').onchange = renderBooks;
$('#refreshUsers').onclick = loadAll;
$('#goalForm').onsubmit = async e => {
    e.preventDefault();
    try {
        const n = Number($('#goalInput').value);
        const d = await gql(`mutation { setYearlyReadingGoal(goal:${n}) { totalCompletedBooks currentlyReading wantToRead readingProgress yearlyReadingGoal completedThisYear yearlyGoalProgress } }`);
        renderStats(d.setYearlyReadingGoal);
        notice('Reading goal updated.')
    } catch (x) {
        notice(x.message, true)
    }
};
$('#profileForm').onsubmit = async e => {
    e.preventDefault();
    const f = new FormData(e.target), v = Object.fromEntries(f);
    const q = `mutation { updateUser(id:"${currentUser.id}",input:{name:${JSON.stringify(v.name)},nick:${JSON.stringify(v.nick)},location:${JSON.stringify(v.location || '')},birthDate:${JSON.stringify(v.birthDate || '')},photo:${JSON.stringify(v.photo || '')}}) { ${userFields} } }`;
    try {
        await gql(q);
        currentUser.name = v.name;
        localStorage.setItem('bookly_user', JSON.stringify(currentUser));
        $('#sideName').textContent = v.name;
        currentUser.photo = v.photo.trim() || null;
        localStorage.setItem('bookly_user', JSON.stringify(currentUser));
        setAvatar(currentUser);
        notice('Profile saved.')
    } catch (x) {
        notice(x.message, true)
    }
};

function openModal(b) {
    const f = $('#bookForm');
    f.reset();
    f.elements.id.value = b?.id || '';
    $('#modalTitle').textContent = b ? 'Edit book' : 'Add a book';
    $('#adminOwnerField').classList.toggle('hidden', currentUser.role !== 'ADMIN' || !!b);
    if (currentUser.role === 'ADMIN' && !b) {
        $('#bookOwner').innerHTML = users.map(u => `<option value="${u.id}">${escapeHTML(u.name)} (@${escapeHTML(u.nick)})</option>`).join('');
        $('#bookOwner').value = selectedUserId || currentUser.id;
    }
    if (b) {
        for (const k of ['title', 'author', 'status', 'rating', 'pageCount', 'currentPage', 'price', 'review']) f.elements[k].value = b[k] ?? (k === 'rating' ? 0 : '');
        f.elements.isFavorite.value = String(b.isFavorite);
        f.elements.quotes.value = (b.quotes || []).join('\n')
    }
    $('#bookModal').classList.remove('hidden')
}

function closeModal() {
    $('#bookModal').classList.add('hidden')
}

$('#addBookBtn').onclick = () => openModal();
$('#closeModal').onclick = $('#cancelBook').onclick = closeModal;
$('#bookModal').onclick = e => {
    if (e.target.id === 'bookModal') closeModal()
};
$('#bookForm').onsubmit = async e => {
    e.preventDefault();
    const f = new FormData(e.target), id = f.get('id'), v = {
        title: f.get('title'),
        author: f.get('author'),
        status: f.get('status'),
        rating: Number(f.get('rating')),
        isFavorite: f.get('isFavorite') === 'true',
        quotes: f.get('quotes').split('\n').map(x => x.trim()).filter(Boolean)
    };
    for (const k of ['pageCount', 'currentPage', 'price']) if (f.get(k) !== '') v[k] = k === 'price' ? Number(f.get(k)) : Number(f.get(k));
    for (const k of ['review']) if (f.get(k)) v[k] = f.get(k);
    const input = Object.entries(v).map(([k, x]) => `${k}:${k === 'status' ? x : JSON.stringify(x)}`).join(',');
    try {
        if (!id && currentUser.role === 'ADMIN') {
            const payload = {...v, userId: f.get('userId')};
            await request('/admin/books', payload)
        } else {
            await gql(id ? `mutation { updateBook(id:"${id}",input:{${input}}) { id } }` : `mutation { createBook(input:{${input}}) { id } }`)
        }
        closeModal();
        await loadAll();
        if (selectedUserId) viewUser(selectedUserId);
        notice(id ? 'Book updated.' : 'Book added successfully.')
    } catch (x) {
        notice(x.message, true)
    }
};
window.editBook = id => {
    const b = books.find(x => x.id === id);
    if (b) openModal(b)
};
window.openAdminBook = id => {
    selectedUserId = id;
    openModal()
};
window.removeBook = async id => {
    if (!confirm('Delete this book? This cannot be undone.')) return;
    try {
        await gql(`mutation { deleteBook(id:"${id}") }`);
        await loadAll();
        if (selectedUserId) viewUser(selectedUserId);
        notice('Book deleted.')
    } catch (e) {
        notice(e.message, true)
    }
};
window.favoriteBook = async id => {
    try {
        await gql(`mutation { toggleFavorite(id:"${id}") { id } }`);
        await loadAll()
    } catch (e) {
        notice(e.message, true)
    }
};
if (token && currentUser) {
    setAuth()
} else logout();

window.removeUser = async id => {
    if (currentUser.role !== 'ADMIN' || !confirm('Delete this user and all books in their library? This cannot be undone.')) return;
    try {
        await gql(`mutation { deleteUser(id:"${id}") }`);
        if (selectedUserId === id) closeUserDetail();
        await loadAll();
        notice('User and their books deleted.')
    } catch (e) {
        notice(e.message, true)
    }
};
