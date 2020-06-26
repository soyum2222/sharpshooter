package sharpshooter

import (
	"fmt"
	"math"
)

func (s *Sniper) calrto(rtt int64) {

	//RFC6298
	if s.rtt == 0 {
		SRTT := rtt
		RTTVAR := rtt / 2
		RTO := SRTT + int64(math.Max(DEFAULT_INIT_RTO_UNIT, float64(4*RTTVAR)))
		s.rto = RTO
		s.rtt = SRTT
		fmt.Println("rtt rto", s.rtt, s.rto)

	}

	RTTVAR := int64((1-0.25)*(float64(rtt)/2)) + int64(0.25*math.Abs(float64(rtt-s.rtt)))

	SRTT := int64((1-0.8)*float64(s.rtt) + 0.8*float64(rtt))

	RTO := SRTT + int64(math.Max(DEFAULT_INIT_RTO_UNIT, float64(4*RTTVAR)))

	s.rtt = SRTT
	s.rto = RTO

	fmt.Println("rtt rto", s.rtt, s.rto)

}
