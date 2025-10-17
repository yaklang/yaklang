package knowledgegraph

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// CreateMockEntities 创建mock实体数据
func CreateMockEntities() []*Entity {
	entities := []*Entity{
		// 人物实体
		{
			ID:          "person_001",
			Name:        "林纳斯·托瓦兹",
			Type:        EntityTypePerson,
			Description: "Linux内核的创始人和主要开发者，Git版本控制系统的创作者。芬兰裔美国软件工程师，被誉为开源软件运动的重要推动者。",
			Aliases:     []string{"Linus Torvalds", "林纳斯", "托瓦兹"},
			Properties: map[string]interface{}{
				"nationality": "Finnish-American",
				"birth_year":  1969,
				"occupation":  "Software Engineer",
				"known_for":   []string{"Linux", "Git"},
				"awards":      []string{"Millennium Technology Prize", "IEEE Computer Pioneer Award"},
			},
			Tags:      []string{"开源", "Linux", "软件工程", "编程"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:          "person_002",
			Name:        "蒂姆·伯纳斯-李",
			Type:        EntityTypePerson,
			Description: "万维网(World Wide Web)的发明者，HTTP协议、HTML语言和URL的创造者。现任万维网联盟(W3C)主席，被誉为互联网之父。",
			Aliases:     []string{"Tim Berners-Lee", "TimBL", "互联网之父"},
			Properties: map[string]interface{}{
				"nationality": "British",
				"birth_year":  1955,
				"occupation":  "Computer Scientist",
				"known_for":   []string{"World Wide Web", "HTTP", "HTML", "URL"},
				"title":       "Director of W3C",
			},
			Tags:      []string{"万维网", "HTTP", "HTML", "W3C", "互联网"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		// 组织实体
		{
			ID:          "org_001",
			Name:        "Google",
			Type:        EntityTypeOrganization,
			Description: "全球领先的搜索引擎和互联网技术公司，业务涵盖搜索、广告、云计算、人工智能等多个领域。开发了Android操作系统、Chrome浏览器等知名产品。",
			Aliases:     []string{"谷歌", "Alphabet Inc.", "Google LLC"},
			Properties: map[string]interface{}{
				"founded":      1998,
				"headquarters": "Mountain View, California",
				"industry":     "Technology",
				"employees":    "over 150,000",
				"products":     []string{"Search", "Android", "Chrome", "YouTube", "Gmail"},
			},
			Tags:      []string{"搜索引擎", "互联网", "人工智能", "云计算", "移动操作系统"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:          "org_002",
			Name:        "OWASP",
			Type:        EntityTypeOrganization,
			Description: "开放式Web应用程序安全项目，致力于改善软件安全的全球性非营利组织。发布OWASP Top 10等重要安全指南，推广Web应用安全最佳实践。",
			Aliases:     []string{"Open Web Application Security Project", "开放式Web应用程序安全项目"},
			Properties: map[string]interface{}{
				"founded":      2001,
				"type":         "Non-profit",
				"focus":        "Web Application Security",
				"publications": []string{"OWASP Top 10", "OWASP Testing Guide", "OWASP Code Review Guide"},
			},
			Tags:      []string{"网络安全", "Web安全", "应用安全", "安全标准", "开源"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		// 技术/工具实体
		{
			ID:          "tech_001",
			Name:        "Docker",
			Type:        EntityTypeTechnology,
			Description: "开源的容器化平台，允许开发者打包应用及其依赖项到轻量级容器中。简化了应用的部署、扩展和管理，是现代DevOps和云原生架构的核心技术。",
			Aliases:     []string{"Docker Engine", "容器化", "Docker平台"},
			Properties: map[string]interface{}{
				"first_release": "2013",
				"language":      "Go",
				"license":       "Apache 2.0",
				"platform":      "Cross-platform",
				"use_cases":     []string{"Application Containerization", "Microservices", "DevOps", "Cloud Deployment"},
			},
			Tags:      []string{"容器化", "DevOps", "云计算", "微服务", "部署"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:          "tech_002",
			Name:        "Kubernetes",
			Type:        EntityTypeTechnology,
			Description: "开源的容器编排平台，用于自动化容器应用的部署、扩展和管理。提供服务发现、负载均衡、存储编排等功能，是云原生应用的标准编排工具。",
			Aliases:     []string{"K8s", "Kube", "容器编排"},
			Properties: map[string]interface{}{
				"first_release": "2014",
				"language":      "Go",
				"license":       "Apache 2.0",
				"developed_by":  "Google",
				"cncf_project":  true,
			},
			Tags:      []string{"容器编排", "云原生", "微服务", "集群管理", "自动化"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		// 漏洞实体
		{
			ID:          "vuln_001",
			Name:        "SQL注入",
			Type:        EntityTypeVulnerability,
			Description: "一种代码注入攻击技术，攻击者通过在应用程序的输入字段中插入恶意SQL代码来操作数据库。可能导致数据泄露、数据篡改或完整系统的接管。",
			Aliases:     []string{"SQL Injection", "SQLi", "数据库注入"},
			Properties: map[string]interface{}{
				"cwe_id":        "CWE-89",
				"cvss_base":     "High",
				"owasp_ranking": 3, // OWASP Top 10 2021
				"attack_vector": "Network",
				"complexity":    "Low",
			},
			Tags:      []string{"Web安全", "数据库安全", "注入攻击", "OWASP Top 10"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:          "vuln_002",
			Name:        "跨站脚本攻击",
			Type:        EntityTypeVulnerability,
			Description: "一种Web应用安全漏洞，攻击者将恶意脚本注入到网页中，当其他用户浏览该页面时执行恶意代码。可分为反射型、存储型和DOM型XSS。",
			Aliases:     []string{"XSS", "Cross-Site Scripting", "脚本注入"},
			Properties: map[string]interface{}{
				"cwe_id":        "CWE-79",
				"cvss_base":     "Medium",
				"owasp_ranking": 7, // OWASP Top 10 2021
				"attack_vector": "Network",
				"types":         []string{"Reflected XSS", "Stored XSS", "DOM XSS"},
			},
			Tags:      []string{"Web安全", "脚本注入", "客户端攻击", "OWASP Top 10"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		// 概念实体
		{
			ID:          "concept_001",
			Name:        "人工智能",
			Type:        EntityTypeConcept,
			Description: "机器模拟人类智能的技术领域，包括学习、推理、感知、理解语言等能力。涵盖机器学习、深度学习、自然语言处理、计算机视觉等多个子领域。",
			Aliases:     []string{"AI", "Artificial Intelligence", "机器智能"},
			Properties: map[string]interface{}{
				"subfields":    []string{"Machine Learning", "Deep Learning", "NLP", "Computer Vision", "Robotics"},
				"applications": []string{"自动驾驶", "语音识别", "图像识别", "推荐系统", "智能助手"},
				"history":      "起源于1950年代",
			},
			Tags:      []string{"人工智能", "机器学习", "深度学习", "智能系统"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:          "concept_002",
			Name:        "区块链",
			Type:        EntityTypeConcept,
			Description: "一种分布式账本技术，通过密码学方法将数据记录在链式结构中，确保数据的不可篡改性和透明性。广泛应用于加密货币、智能合约等领域。",
			Aliases:     []string{"Blockchain", "分布式账本", "区块链技术"},
			Properties: map[string]interface{}{
				"key_features": []string{"去中心化", "不可篡改", "透明性", "共识机制"},
				"consensus":    []string{"Proof of Work", "Proof of Stake", "Delegated Proof of Stake"},
				"applications": []string{"加密货币", "智能合约", "供应链管理", "数字身份"},
			},
			Tags:      []string{"区块链", "加密货币", "分布式系统", "密码学"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		// 产品实体
		{
			ID:          "product_001",
			Name:        "ChatGPT",
			Type:        EntityTypeProduct,
			Description: "OpenAI开发的大型语言模型聊天机器人，基于GPT架构训练。能够进行自然语言对话、回答问题、协助写作、编程等多种任务。",
			Aliases:     []string{"Chat GPT", "OpenAI ChatGPT", "GPT聊天机器人"},
			Properties: map[string]interface{}{
				"developer":     "OpenAI",
				"launch_date":   "November 2022",
				"model_type":    "Large Language Model",
				"capabilities":  []string{"对话", "问答", "写作", "编程", "翻译", "总结"},
				"pricing_model": "Freemium",
			},
			Tags:      []string{"人工智能", "语言模型", "聊天机器人", "OpenAI", "自然语言处理"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},

		// 事件实体
		{
			ID:          "event_001",
			Name:        "WannaCry勒索软件攻击",
			Type:        EntityTypeEvent,
			Description: "2017年5月发生的全球性勒索软件攻击事件，利用NSA泄露的Windows漏洞EternalBlue传播。影响了150多个国家的数十万台计算机，包括医院、政府机构等重要设施。",
			Aliases:     []string{"WannaCry攻击", "WannaCrypt", "想哭病毒"},
			Properties: map[string]interface{}{
				"date":               "May 12, 2017",
				"type":               "Ransomware Attack",
				"exploit_used":       "EternalBlue",
				"affected_countries": 150,
				"estimated_damage":   "billions of dollars",
				"attribution":        "Lazarus Group (suspected)",
			},
			Tags:      []string{"勒索软件", "网络攻击", "网络安全事件", "恶意软件"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	return entities
}

// GenerateRandomEntities 生成随机实体数据用于测试
func GenerateRandomEntities(count int) []*Entity {
	entityTypes := []EntityType{
		EntityTypePerson,
		EntityTypeOrganization,
		EntityTypeTechnology,
		EntityTypeVulnerability,
		EntityTypeConcept,
		EntityTypeProduct,
	}

	var entities []*Entity
	for i := 0; i < count; i++ {
		entityType := entityTypes[i%len(entityTypes)]

		entity := &Entity{
			ID:          fmt.Sprintf("random_%d", i),
			Name:        fmt.Sprintf("Random %s %d", entityType, i),
			Type:        entityType,
			Description: fmt.Sprintf("This is a randomly generated %s entity for testing purposes. ID: %d", entityType, i),
			Aliases:     []string{fmt.Sprintf("alias_%d", i), fmt.Sprintf("alt_%d", i)},
			Properties: map[string]interface{}{
				"test_id":    i,
				"generated":  true,
				"random_key": utils.RandStringBytes(10),
			},
			Tags:      []string{"test", "random", string(entityType)},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		entities = append(entities, entity)
	}

	return entities
}
