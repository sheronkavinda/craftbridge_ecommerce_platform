-- Grant the seller role to owners of active shops with approved applications.
INSERT INTO user_roles (user_id, role_id)
SELECT DISTINCT s.owner_user_id, (SELECT id FROM roles WHERE name = 'seller')
FROM shops AS s
JOIN seller_applications AS a
  ON a.id = s.application_id AND a.user_id = s.owner_user_id
WHERE a.status = 'approved' AND s.status = 'active'
ON CONFLICT (user_id, role_id) DO NOTHING;
