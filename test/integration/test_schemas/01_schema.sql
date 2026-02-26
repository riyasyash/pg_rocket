-- Simple parent-child schema for testing
CREATE TABLE organizations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    manager_id INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (org_id) REFERENCES organizations(id),
    FOREIGN KEY (manager_id) REFERENCES users(id)
);

CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (org_id) REFERENCES organizations(id)
);

CREATE TABLE tasks (
    id SERIAL PRIMARY KEY,
    project_id INTEGER NOT NULL,
    assigned_to INTEGER,
    title VARCHAR(255) NOT NULL,
    status VARCHAR(50),
    tags TEXT[],
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (assigned_to) REFERENCES users(id)
);

CREATE TABLE comments (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Insert test data
INSERT INTO organizations (id, name) VALUES
    (1, 'Acme Corp'),
    (2, 'Tech Startup'),
    (3, 'Enterprise Inc');

INSERT INTO users (id, org_id, name, email, manager_id) VALUES
    (1, 1, 'Alice Admin', 'alice@acme.com', NULL),
    (2, 1, 'Bob Builder', 'bob@acme.com', 1),
    (3, 1, 'Charlie Coder', 'charlie@acme.com', 1),
    (4, 2, 'Diana Dev', 'diana@startup.com', NULL),
    (5, 2, 'Eve Engineer', 'eve@startup.com', 4);

INSERT INTO projects (id, org_id, name, description, metadata) VALUES
    (1, 1, 'Project Alpha', 'First project', '{"priority": "high", "budget": 100000}'),
    (2, 1, 'Project Beta', 'Second project', '{"priority": "medium", "budget": 50000}'),
    (3, 2, 'Startup MVP', 'Minimum viable product', '{"priority": "critical", "stage": "development"}');

INSERT INTO tasks (id, project_id, assigned_to, title, status, tags) VALUES
    (1, 1, 2, 'Setup infrastructure', 'completed', ARRAY['devops', 'infrastructure']),
    (2, 1, 3, 'Implement auth', 'in-progress', ARRAY['backend', 'security']),
    (3, 1, 3, 'Write tests', 'pending', ARRAY['testing']),
    (4, 2, 2, 'Design UI', 'in-progress', ARRAY['frontend', 'design']),
    (5, 3, 5, 'Build API', 'completed', ARRAY['backend', 'api']);

INSERT INTO comments (id, task_id, user_id, content) VALUES
    (1, 1, 1, 'Great work on the infrastructure setup!'),
    (2, 1, 2, 'Thanks! I used Docker for everything.'),
    (3, 2, 1, 'Make sure to use bcrypt for passwords.'),
    (4, 2, 3, 'Will do! Already implemented.'),
    (5, 4, 2, 'I''m thinking of using Tailwind CSS.');

-- Reset sequences to match inserted data
SELECT setval('organizations_id_seq', 3);
SELECT setval('users_id_seq', 5);
SELECT setval('projects_id_seq', 3);
SELECT setval('tasks_id_seq', 5);
SELECT setval('comments_id_seq', 5);
