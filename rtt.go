package sharpshooter

import (
	"fmt"
	"time"
)

func (s *Sniper) calrto(rtt int64) {

	//RFC6298
	if s.rtt == 0 {
		SRTT := rtt
		//RTTVAR := rtt / 2
		//RTO := SRTT + int64(math.Max(DEFAULT_INIT_RTO_UNIT, float64(4*RTTVAR)))
		RTO := rtt * 2
		s.rto = RTO
		s.rtt = SRTT
	}

	//RTTVAR := int64((1-0.25)*(float64(rtt)/2)) + int64(0.25*math.Abs(float64(rtt-s.rtt)))
	//
	//SRTT := int64((1-0.8)*float64(s.rtt) + 0.8*float64(rtt))
	//
	//RTO := SRTT + int64(math.Max(DEFAULT_INIT_RTO_UNIT, float64(4*RTTVAR)))
	if rtt > s.rtt*2 {
		return
	}

	s.rtt = int64((0.3)*float64(rtt)) + int64((0.7)*float64(s.rtt))
	s.rto = s.rtt * 2

	if s.rtt < int64(time.Millisecond*10) {
		s.rtt = int64(time.Millisecond * 10)
	}

	fmt.Printf("rtt %d s.rtt %d \n", rtt, s.rtt)
}
