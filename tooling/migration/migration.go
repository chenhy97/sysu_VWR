// Package migration implements a simulation of migration to a global large scale microservice architecture
// It creates and controls a collection of aws, lamp, netflixoss and netflix application microservices
// or reads in a network from a json file. It also logs the architecture (nodes and links) as it evolves
package migration

import (
	. "github.com/chenhy/spigo/actors/packagenames" // name definitions
	"github.com/chenhy/spigo/tooling/archaius"      // global configuration
	"github.com/chenhy/spigo/tooling/asgard"        // tools to create an architecture
	"log"
)

// Start lamp to netflixoss step by step migration
func Start() {
	regions := archaius.Conf.Regions
	var ServiceName map[int]string
	var ServiceIndex int
	if archaius.Conf.Population < 1 {
		log.Fatal("migration: can't create less than 1 microservice")
	} else {
		log.Printf("migration: scaling to %v%%", archaius.Conf.Population)
	}
	// Build the configuration step by step
	// start mysql data store layer, which connects to itself
	mysqlcount := 2
	sname := "rds-mysql"
	// start memcached layer, only one per region
	mname := "memcache"
	mcount := 1
	if archaius.Conf.StopStep >= 3 {
		// start evcache layer, one per zone
		mname = "evcache"
		mcount = 3
	}
	// priam managed Cassandra cluster, turtle because it's used to configure other clusters
	priamCassandracount := 12 * archaius.Conf.Population / 100
	cname := "cassTurtle"
	// staash data access layer connects to mysql master and slave, and evcache
	staashcount := 6 * archaius.Conf.Population / 100
	tname := "turtle"
	//  php business logic, we can create a network of simple services from the karyon package
	phpcount := 9 * archaius.Conf.Population / 100
	pname := "php"
	// some node microservice logic, we can create a network of simple services from the karyon package
	nodecount := 9 * archaius.Conf.Population / 100
	nname := "node"
	// zuul api proxies and insert between elb and php
	zuulcount := 9 * archaius.Conf.Population / 100
	zuname := "wwwproxy"
	// AWS elastic load balancer
	elbname := "www-elb"
	// DNS endpoint
	dns := "www"
	switch archaius.Conf.StopStep {
	case 0: // basic LAMP
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(sname, StorePkg, regions, mysqlcount, sname)
		asgard.Create(pname, MonolithPkg, regions, phpcount, sname)
		asgard.Create(elbname, ElbPkg, regions, 0, pname)
		ServiceIndex = 3
		ServiceName[0] = sname
		ServiceName[1] = pname
		ServiceName[2] = elbname
	case 1: // basic LAMP with memcache
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(sname, StorePkg, regions, mysqlcount, sname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(pname, MonolithPkg, regions, phpcount, sname, mname)
		asgard.Create(elbname, ElbPkg, regions, 0, pname)
		ServiceIndex = 4
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = pname
		ServiceName[3] = elbname
	case 2: // LAMP with zuul and memcache
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(sname, StorePkg, regions, mysqlcount, sname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(pname, MonolithPkg, regions, phpcount, sname, mname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 5
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = pname
		ServiceName[3] = zuname
		ServiceName[4] = elbname
	case 3: // LAMP with zuul and staash and evcache
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(sname, StorePkg, regions, mysqlcount, sname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, sname, mname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 6
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = tname
		ServiceName[3] = pname
		ServiceName[4] = zuname
		ServiceName[5] = elbname
	case 4: // added node microservice
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(sname, StorePkg, regions, mysqlcount, sname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, sname, mname, cname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(nname, KaryonPkg, regions, nodecount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname, nname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 7
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = tname
		ServiceName[3] = pname
		ServiceName[4] = nname
		ServiceName[5] = zuname
		ServiceName[6] = elbname
	case 5: // added cassandra alongside mysql
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(cname, PriamCassandraPkg, regions, priamCassandracount, cname)
		asgard.Create(sname, StorePkg, regions, mysqlcount, sname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, sname, mname, cname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(nname, KaryonPkg, regions, nodecount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname, nname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 8
		ServiceName[0] = cname
		ServiceName[1] = sname
		ServiceName[2] = mname
		ServiceName[3] = tname
		ServiceName[4] = pname
		ServiceName[5] = nname
		ServiceName[6] = zuname
		ServiceName[7] = elbname
	case 6: // removed mysql so that multi-region will work properly
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(cname, PriamCassandraPkg, regions, priamCassandracount, cname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, mname, cname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(nname, KaryonPkg, regions, nodecount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname, nname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 7
		ServiceName[0] = cname
		ServiceName[1] = mname
		ServiceName[2] = tname
		ServiceName[3] = pname
		ServiceName[4] = nname
		ServiceName[5] = zuname
		ServiceName[6] = elbname
	case 7: // set two regions with disconnected priamCassandra
		regions = 2
		archaius.Conf.Regions = regions
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(cname, PriamCassandraPkg, regions, priamCassandracount, cname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, mname, cname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(nname, KaryonPkg, regions, nodecount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname, nname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 7
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = tname
		ServiceName[3] = pname
		ServiceName[4] = nname
		ServiceName[5] = zuname
		ServiceName[6] = elbname
	case 8: // set two regions with connected priamCassandra
		regions = 2
		archaius.Conf.Regions = regions
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(cname, PriamCassandraPkg, regions, priamCassandracount, "eureka", cname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, mname, cname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(nname, KaryonPkg, regions, nodecount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname, nname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 7
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = tname
		ServiceName[3] = pname
		ServiceName[4] = nname
		ServiceName[5] = zuname
		ServiceName[6] = elbname
	case 9: // set three regions with disconnected priamCassandra
		regions = 3
		archaius.Conf.Regions = regions
		asgard.CreateChannels()
		asgard.CreateEureka() // service registries for each zone
		asgard.Create(cname, PriamCassandraPkg, regions, priamCassandracount, "eureka", cname)
		asgard.Create(mname, StorePkg, regions, mcount)
		asgard.Create(tname, StaashPkg, regions, staashcount, mname, cname)
		asgard.Create(pname, KaryonPkg, regions, phpcount, tname)
		asgard.Create(nname, KaryonPkg, regions, nodecount, tname)
		asgard.Create(zuname, ZuulPkg, regions, zuulcount, pname, nname)
		asgard.Create(elbname, ElbPkg, regions, 0, zuname)
		ServiceIndex = 7
		ServiceName[0] = sname
		ServiceName[1] = mname
		ServiceName[2] = tname
		ServiceName[3] = pname
		ServiceName[4] = nname
		ServiceName[5] = zuname
		ServiceName[6] = elbname
	}
	dnsname := asgard.Create(dns, DenominatorPkg, 0, 0, elbname)
	asgard.Run(dnsname,ServiceName,ServiceIndex)
}
