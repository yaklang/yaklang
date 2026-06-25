const express = require('express');
const router = express.Router();

router.get('/api/orders', (req, res) => res.json([]));
router.post('/api/orders', (req, res) => res.json({created: true}));
router.delete('/api/orders/:id', (req, res) => res.json({deleted: true}));

module.exports = router;
